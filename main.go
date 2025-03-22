package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/withObsrvr/pluginapi"
)

// ErrorType defines the category of an error
type ErrorType string

const (
	// Configuration errors
	ErrorTypeConfig ErrorType = "config"
	// Data parsing errors
	ErrorTypeParsing ErrorType = "parsing"
	// Event processing errors
	ErrorTypeProcessing ErrorType = "processing"
	// Consumer-related errors
	ErrorTypeConsumer ErrorType = "consumer"
)

// ErrorSeverity defines how critical an error is
type ErrorSeverity string

const (
	// Fatal errors that should stop processing
	ErrorSeverityFatal ErrorSeverity = "fatal"
	// Errors that can be logged but processing can continue
	ErrorSeverityWarning ErrorSeverity = "warning"
	// Informational issues that might be useful for debugging
	ErrorSeverityInfo ErrorSeverity = "info"
)

// ProcessorError represents a structured error with context
type ProcessorError struct {
	// Original error
	Err error
	// Type categorizes the error
	Type ErrorType
	// Severity indicates how critical the error is
	Severity ErrorSeverity
	// Transaction hash related to the error, if applicable
	TransactionHash string
	// Ledger sequence related to the error, if applicable
	LedgerSequence uint32
	// Contract ID related to the error, if applicable
	ContractID string
	// Additional context as key-value pairs
	Context map[string]interface{}
}

// Error satisfies the error interface
func (e *ProcessorError) Error() string {
	contextStr := ""
	for k, v := range e.Context {
		contextStr += fmt.Sprintf(" %s=%v", k, v)
	}

	idInfo := ""
	if e.TransactionHash != "" {
		idInfo += fmt.Sprintf(" tx=%s", e.TransactionHash)
	}
	if e.LedgerSequence > 0 {
		idInfo += fmt.Sprintf(" ledger=%d", e.LedgerSequence)
	}
	if e.ContractID != "" {
		idInfo += fmt.Sprintf(" contract=%s", e.ContractID)
	}

	return fmt.Sprintf("[%s:%s]%s%s: %v", e.Type, e.Severity, idInfo, contextStr, e.Err)
}

// Unwrap returns the original error
func (e *ProcessorError) Unwrap() error {
	return e.Err
}

// IsFatal returns true if the error is fatal
func (e *ProcessorError) IsFatal() bool {
	return e.Severity == ErrorSeverityFatal
}

// NewProcessorError creates a new processor error
func NewProcessorError(err error, errType ErrorType, severity ErrorSeverity) *ProcessorError {
	return &ProcessorError{
		Err:      err,
		Type:     errType,
		Severity: severity,
		Context:  make(map[string]interface{}),
	}
}

// WithTransaction adds transaction information to the error
func (e *ProcessorError) WithTransaction(hash string) *ProcessorError {
	e.TransactionHash = hash
	return e
}

// WithLedger adds ledger information to the error
func (e *ProcessorError) WithLedger(sequence uint32) *ProcessorError {
	e.LedgerSequence = sequence
	return e
}

// WithContract adds contract information to the error
func (e *ProcessorError) WithContract(id string) *ProcessorError {
	e.ContractID = id
	return e
}

// WithContext adds additional context to the error
func (e *ProcessorError) WithContext(key string, value interface{}) *ProcessorError {
	e.Context[key] = value
	return e
}

// ContractEvent represents an event emitted by a contract with enhanced structure
type ContractEvent struct {
	// Transaction context
	Timestamp         time.Time `json:"timestamp"`
	LedgerSequence    uint32    `json:"ledger_sequence"`
	TransactionHash   string    `json:"transaction_hash"`
	TransactionID     int64     `json:"transaction_id,omitempty"`
	Successful        bool      `json:"successful"`
	NetworkPassphrase string    `json:"network_passphrase"`

	// Event context
	ContractID         string `json:"contract_id"`
	Type               string `json:"type"`
	TypeCode           int32  `json:"type_code,omitempty"`
	EventIndex         int    `json:"event_index"`
	OperationIndex     int    `json:"operation_index"`
	InSuccessfulTxCall bool   `json:"in_successful_tx_call"`

	// Event data
	Topics        []TopicData     `json:"topics,omitempty"`
	TopicsDecoded []TopicData     `json:"topics_decoded,omitempty"`
	Data          json.RawMessage `json:"data"`
	DataDecoded   json.RawMessage `json:"data_decoded,omitempty"`

	// Raw XDR for archival and debugging
	EventXDR string `json:"event_xdr,omitempty"`

	// Additional diagnostic data
	DiagnosticEvents []DiagnosticData `json:"diagnostic_events,omitempty"`

	// Metadata for querying and filtering
	Tags map[string]string `json:"tags,omitempty"`
}

// TopicData represents a structured topic with type information
type TopicData struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// DiagnosticData contains additional diagnostic information about an event
type DiagnosticData struct {
	Event                    json.RawMessage `json:"event"`
	InSuccessfulContractCall bool            `json:"in_successful_contract_call"`
	RawXDR                   string          `json:"raw_xdr,omitempty"`
}

// ContractFilterProcessor handles filtering of contract events based on contract ID
type ContractFilterProcessor struct {
	consumers  []pluginapi.Consumer
	contractID string
	mu         sync.RWMutex
	stats      struct {
		ProcessedEvents uint64
		FilteredEvents  uint64
		LastEventTime   time.Time
		ErrorCount      uint64
	}
}

func (p *ContractFilterProcessor) Initialize(config map[string]interface{}) error {
	contractID, ok := config["contract_id"].(string)
	if !ok {
		return NewProcessorError(
			errors.New("contract_id must be specified"),
			ErrorTypeConfig,
			ErrorSeverityFatal,
		)
	}

	if contractID == "" {
		return NewProcessorError(
			errors.New("contract_id cannot be empty"),
			ErrorTypeConfig,
			ErrorSeverityFatal,
		)
	}

	p.contractID = contractID
	log.Printf("ContractFilterProcessor: Initialized with contract ID: %s", contractID)
	return nil
}

func (p *ContractFilterProcessor) RegisterConsumer(consumer pluginapi.Consumer) {
	log.Printf("ContractFilterProcessor: Registering consumer %s", consumer.Name())
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consumers = append(p.consumers, consumer)
}

func (p *ContractFilterProcessor) Close() error {
	return nil
}

func (p *ContractFilterProcessor) Process(ctx context.Context, msg pluginapi.Message) error {
	// Check for canceled context before starting work
	if err := ctx.Err(); err != nil {
		return NewProcessorError(
			fmt.Errorf("context canceled before processing: %w", err),
			ErrorTypeProcessing,
			ErrorSeverityFatal,
		)
	}

	log.Printf("ContractFilterProcessor: received message payload type: %T", msg.Payload)

	var event ContractEvent
	switch payload := msg.Payload.(type) {
	case []byte:
		if err := json.Unmarshal(payload, &event); err != nil {
			procErr := NewProcessorError(
				fmt.Errorf("error decoding event: %w", err),
				ErrorTypeParsing,
				ErrorSeverityWarning,
			)
			log.Printf("Warning: %s", procErr.Error())

			p.mu.Lock()
			p.stats.ErrorCount++
			p.mu.Unlock()

			return nil
		}
	case ContractEvent:
		event = payload
	default:
		procErr := NewProcessorError(
			fmt.Errorf("unexpected payload type: %T", msg.Payload),
			ErrorTypeParsing,
			ErrorSeverityWarning,
		)
		log.Printf("Warning: %s", procErr.Error())

		p.mu.Lock()
		p.stats.ErrorCount++
		p.mu.Unlock()

		return nil
	}

	log.Printf("ContractFilterProcessor: encountered event with contractID: %s", event.ContractID)

	p.mu.Lock()
	p.stats.ProcessedEvents++
	p.stats.LastEventTime = time.Now()
	p.mu.Unlock()

	// Apply contract ID filter
	if event.ContractID != p.contractID {
		log.Printf("ContractFilterProcessor: event contractID %s does not match filter %s; skipping", event.ContractID, p.contractID)
		return nil
	}

	p.mu.Lock()
	p.stats.FilteredEvents++
	p.mu.Unlock()

	log.Printf("ContractFilterProcessor: processing filtered event for contract %s", p.contractID)

	// Forward to consumers with improved error handling and context management
	return p.forwardToConsumers(ctx, msg, event)
}

func (p *ContractFilterProcessor) forwardToConsumers(ctx context.Context, msg pluginapi.Message, event ContractEvent) error {
	// Check for context cancellation
	if err := ctx.Err(); err != nil {
		return NewProcessorError(
			fmt.Errorf("context canceled before forwarding: %w", err),
			ErrorTypeConsumer,
			ErrorSeverityWarning,
		)
	}

	// Lock to safely access p.consumers
	p.mu.RLock()
	consumers := make([]pluginapi.Consumer, len(p.consumers))
	copy(consumers, p.consumers)
	p.mu.RUnlock()

	log.Printf("ContractFilterProcessor: Forwarding event to %d consumers", len(consumers))

	for _, consumer := range consumers {
		// Check context before each consumer to allow early exit
		if err := ctx.Err(); err != nil {
			return NewProcessorError(
				fmt.Errorf("context canceled during forwarding: %w", err),
				ErrorTypeConsumer,
				ErrorSeverityWarning,
			).WithContract(event.ContractID)
		}

		log.Printf("ContractFilterProcessor: Forwarding to consumer: %s", consumer.Name())

		// Create a consumer-specific timeout context
		consumerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := consumer.Process(consumerCtx, msg)
		cancel() // Always cancel to prevent context leak

		if err != nil {
			procErr := NewProcessorError(
				fmt.Errorf("error in consumer %s: %w", consumer.Name(), err),
				ErrorTypeConsumer,
				ErrorSeverityWarning,
			).WithContext("consumer", consumer.Name()).
				WithContract(event.ContractID)

			log.Printf("Error forwarding event: %s", procErr.Error())

			p.mu.Lock()
			p.stats.ErrorCount++
			p.mu.Unlock()

			return procErr
		}
	}

	return nil
}

func (p *ContractFilterProcessor) Name() string {
	return "flow/processor/contract-filter"
}

func (p *ContractFilterProcessor) Version() string {
	return "1.0.0"
}

func (p *ContractFilterProcessor) Type() pluginapi.PluginType {
	return pluginapi.ProcessorPlugin
}

// New creates a new instance of the processor
func New() pluginapi.Plugin {
	return &ContractFilterProcessor{}
}
