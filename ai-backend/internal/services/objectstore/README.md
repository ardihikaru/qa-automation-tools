# services/objectstore

## Overview
A minimal S3/MinIO client for the module checklist export cache ([MODULE_CHECKLIST_EXPORT.md](../../../../docs/designs/MODULE_CHECKLIST_EXPORT.md)). The bucket is the staging MinIO that SUT itself uses, so the package is locked to the key prefix `qa-automation/` and offers no way to list or delete. The only caller is `knowledge.ExportCache`.

## Interface
- `Store` (`objectstore.go`): `Stat(ctx, key) (Info, bool, error)`, `Get(ctx, key) ([]byte, bool, error)` (a missing object is `(nil, false, nil)`), `Put(ctx, key, contentType, data) error` (overwrites). **No `List`, no `Delete`.**
- `Prefix = "qa-automation/"`, `MaxGetBytes` (64 MiB cap on `Get`), `Info{Size, LastModified}`.
- `New(config.ObjectStoreConfig) (*MinioStore, error)` (`minio.go`): runs `ObjectStoreConfig.Check`; returns a nil store and an error when unset or invalid. The connection is lazy.
- `OpError{Op, Code}`, `ErrCode(err)`, `ErrOp(err)`: failures reduced to an operation and an S3 error code (or the Go type of a non-S3 error). `ErrInvalidKey`.

## Invariants
- Every method validates the key **before any network call** (`validateKey`): it must start with the prefix, have a non-empty remainder, no empty segment (`//`), no `.` or `..` segment, no backslash, no control character, and not begin with `/`. The constructor refuses an empty prefix or one without a trailing `/`.
- Each call runs under the configured per-call timeout (`MINIO_TIMEOUT_SECONDS`, default 30 s).
- Errors never carry the endpoint, bucket, key or the remote message; the minio client is an unexported field. The config's `String`/`GoString` print `***` for both secrets.
- The package never creates buckets and never reads outside its keys.

## Side Effects
Outbound HTTP(S) to `MINIO_ENDPOINT` on first use. `Put` writes objects under `qa-automation/<namespace>/...` and silently overwrites an existing key.

## Verification
`go test -mod=mod ./internal/services/objectstore/`: key validation table, constructor, "no List/Delete" (reflection on `Store` and `MinioStore`), no network call for an invalid key, not-found mapping, error redaction and the size cap, over an in-memory fake (`fake_test.go`). Not covered without credentials: a call to a real MinIO (staging check, see the spec's Verification Plan).
