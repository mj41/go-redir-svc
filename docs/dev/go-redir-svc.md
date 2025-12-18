# Development: go-redir-svc

## Local Run

To run the service locally for testing:

```bash
go run main.go --config config.yaml --log-dir ./logs
```

## Build & Push

Commands for building and pushing the container image:

```bash
make build-image
make push-image
```

## Implementation Details

- **Redirect Logic**: The core logic is in `handleRedirect` in `main.go`.
- **Configuration**: The service uses `gopkg.in/yaml.v3` to parse `config.yaml`.
- **Logging**: JSONL logs are written using a mutex-protected file writer.
