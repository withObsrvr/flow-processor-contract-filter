package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/stellar/go/xdr"
	"github.com/withObsrvr/pluginapi"
)

// ContractEvent represents an event emitted by a contract
type ContractEvent struct {
	Timestamp         time.Time        `json:"timestamp"`
	LedgerSequence    uint32           `json:"ledger_sequence"`
	TransactionHash   string           `json:"transaction_hash"`
	ContractID        string           `json:"contract_id"`
	Type              string           `json:"type"`
	Topic             []xdr.ScVal      `json:"topic"`
	Data              json.RawMessage  `json:"data"`
	InSuccessfulTx    bool             `json:"in_successful_tx"`
	EventIndex        int              `json:"event_index"`
	OperationIndex    int              `json:"operation_index"`
	DiagnosticEvents  []DiagnosticData `json:"diagnostic_events,omitempty"`
	NetworkPassphrase string           `json:"network_passphrase"`
}

type DiagnosticData struct {
	Event                    json.RawMessage `json:"event"`
	InSuccessfulContractCall bool            `json:"in_successful_contract_call"`
}

type ContractFilterProcessor struct {
	consumers  []pluginapi.Consumer
	contractID string
	mu         sync.RWMutex
	stats      struct {
		ProcessedEvents uint64
		FilteredEvents  uint64
		LastEventTime   time.Time
	}
}

func (p *ContractFilterProcessor) Initialize(config map[string]interface{}) error {
	contractID, ok := config["contract_id"].(string)
	if !ok {
		return fmt.Errorf("contract_id must be specified")
	}
	p.contractID = contractID
	return nil
}

func (p *ContractFilterProcessor) RegisterConsumer(consumer pluginapi.Consumer) {
	log.Printf("ContractFilterProcessor: Registering consumer %s", consumer.Name())
	p.consumers = append(p.consumers, consumer)
}

func (p *ContractFilterProcessor) Close() error {
	return nil
}

func (p *ContractFilterProcessor) Process(ctx context.Context, msg pluginapi.Message) error {
	log.Printf("ContractFilterProcessor: received message payload type: %T", msg.Payload)

	var event ContractEvent
	switch payload := msg.Payload.(type) {
	case []byte:
		if err := json.Unmarshal(payload, &event); err != nil {
			log.Printf("ContractFilterProcessor: error decoding event: %v", err)
			return nil
		}
	case ContractEvent:
		event = payload
	default:
		log.Printf("ContractFilterProcessor: unexpected payload type: %T", msg.Payload)
		return nil
	}

	log.Printf("ContractFilterProcessor: encountered event with contractID: %s", event.ContractID)

	p.mu.Lock()
	p.stats.ProcessedEvents++
	p.stats.LastEventTime = time.Now()
	p.mu.Unlock()

	if event.ContractID != p.contractID {
		log.Printf("ContractFilterProcessor: event contractID %s does not match filter %s; skipping", event.ContractID, p.contractID)
		return nil
	}

	p.mu.Lock()
	p.stats.FilteredEvents++
	p.mu.Unlock()

	log.Printf("ContractFilterProcessor: processing filtered event for contract %s", p.contractID)

	// Forward to consumers
	for _, consumer := range p.consumers {
		if err := consumer.Process(ctx, msg); err != nil {
			return fmt.Errorf("error in consumer %s: %w", consumer.Name(), err)
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
