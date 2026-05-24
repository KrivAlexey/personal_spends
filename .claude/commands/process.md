# /process

Sends a sample file through the local dev server to test the full pipeline end-to-end.

Usage: `/process <filepath>`

```bash
curl -X POST http://localhost:8080/uploads/csv \
  -H "X-API-Key: $API_KEY" \
  -F "file=@$ARGUMENTS"
```

For images:
```bash
curl -X POST http://localhost:8080/uploads/image \
  -H "X-API-Key: $API_KEY" \
  -F "file=@$ARGUMENTS"
```

The server must be running (`go run ./cmd/server`) before calling this.
