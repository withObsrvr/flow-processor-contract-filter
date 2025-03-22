# flow-processor-contract-filter

A Flow processor plugin that filters contract events based on the specified contract ID.

## Overview

This processor receives contract events and filters them based on a configured contract ID, forwarding only matching events to registered consumers.

## Configuration

The processor requires the following configuration parameter:

- `contract_id`: The contract ID to filter events by (required)

## Building

### Standard Go Build

```bash
go build -buildmode=plugin -o flow-processor-contract-filter.so .
```

## Building with Nix

This project uses Nix for reproducible builds.

### Prerequisites

- Install the [Nix package manager](https://nixos.org/download.html) with flakes enabled.

### Building

1. Clone the repository:
   ```bash
   git clone https://github.com/withObsrvr/flow-processor-contract-filter.git
   cd flow-processor-contract-filter
   ```

2. Build with Nix:
   ```bash
   nix build
   ```

   The built plugin will be located at `./result/lib/flow-processor-contract-filter.so`.

### Development

Enter a development shell with all necessary tools using:

```bash
nix develop
```

This shell automatically vendors dependencies if needed and provides the proper environment for building the plugin.

## Usage

The processor implements the OBSRVR Flow plugin interface and can be loaded by the Flow runtime.

When loaded, it will:
1. Filter incoming contract events based on the configured contract ID
2. Forward matching events to all registered consumers
3. Log information about processed and filtered events