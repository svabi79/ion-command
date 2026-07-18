# ION COMMAND Collector

The collector acquires raw feeds, delegates domain meaning to normalizers, and
publishes the versioned canonical geospatial contract. It is intentionally
independent of Unreal rendering.

```powershell
go test ./...
go run ./cmd/ion-collector -config ./configs/development.json
```

Source and domain interfaces are in `internal/plugins`; the PSKReporter package
is an injection boundary awaiting the subscribed transport/framing details.

