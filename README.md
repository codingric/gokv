# gokv

Simple golang wrapper to sqlite3 to expose key/value store rest api.

## Create a bucket

```bash
curl -X POST http://localhost:8080/bucket \
  -H "Content-Type: application/json" \
  -d '{"email": "test@example.com"}'\


{
    "bucket_id": "a1b2c3d4-e5f6-a7b8-c9d0-e1f2a3b4c5d6",
    "token": "f1e2d3c4-b5a6-f7e8-d9c0-b1a2f3e4d5c6"
}

```

## Create or update a key

```bash
curl -X POST http://localhost:8080/kv/123 -H "Authorization:Bearer f1e2d3c4-b5a6-f7e8-d9c0-b1a2f3e4d5c6" -d "wow"
```

## Read a key

```bash
curl http://localhost:8080/kv/123 -H "Authorization:Bearer f1e2d3c4-b5a6-f7e8-d9c0-b1a2f3e4d5c6"

wow
```
