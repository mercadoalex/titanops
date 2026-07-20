# TitanOps Event Schema (Protobuf)

This directory contains the canonical event schema for the TitanOps platform. All modules MUST emit events conforming to this schema when publishing to the NATS event bus.

## Schema Registry

The schema is published to the [Buf Schema Registry](https://buf.build) at:

```
buf.build/titanops-platform/titanops-events
```

Module repos depend on this schema for code generation and breaking change detection.

## Setup

### Install Buf CLI

```bash
brew install bufbuild/buf/buf
```

### Authenticate with BSR

```bash
buf registry login
```

This will prompt for your buf.build username and token. Generate a token at https://buf.build/settings/user.

### Push Schema to BSR (first time)

```bash
cd proto
buf dep update          # Resolve dependencies (googleapis)
buf lint                # Verify schema correctness
buf push                # Publish to buf.build/mercadoalex/titanops-events
```

### Detect Breaking Changes (CI)

```bash
buf breaking --against 'https://github.com/mercadoalex/titanops.git#branch=main,subdir=proto'
```

This fails if the PR introduces backward-incompatible changes (removed fields, changed types, renumbered fields).

### Generate Go Code

```bash
buf generate
```

Output goes to `../gen/proto/go/titanops/events/v1/`.

## How Module Repos Depend on This Schema

Each module repo adds a `buf.yaml` with a dependency:

```yaml
# In module repo: proto/buf.yaml
version: v2
deps:
  - buf.build/titanops-platform/titanops-events
```

Then:
```bash
buf dep update    # Downloads the schema
buf generate      # Generates typed Go/Rust code from the schema
```

## Schema Evolution Rules

1. **Never remove or renumber existing fields** — use `reserved` instead
2. **Never change a field's type** — add a new field with the new type
3. **New fields must be `optional`** or have default-safe zero values
4. **New enum values** can be added freely (existing code ignores unknown values)
5. **New messages** can be added freely

Breaking changes are detected automatically by `buf breaking` in CI.

## File Structure

```
proto/
├── buf.yaml              # Module config (name, lint rules, breaking rules)
├── buf.gen.yaml          # Code generation config
├── buf.lock              # Locked dependency versions (auto-generated)
├── README.md             # This file
└── titanops/
    └── events/
        └── v1/
            └── events.proto  # The canonical event schema
```
