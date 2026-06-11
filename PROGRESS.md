# Progress Log

## Completed

- [x] Phase 0: Automated Deployment (commit: 88d5ca5)
- [x] Phase 1: Core Server — Go + SQLite + Filesystem (commit: pending)

## Current Iteration

- Iteration: 4
- Working on: Review Phase 1 fix — blob shard path & dead code
- Started: 2026-06-11

## Last Completed

- Fix 1: Blob path shard mismatch (commit: 71e7720)
  - Changed `InsertFile` call in `server/handlers/files.go` from `fileID+".enc"` to `fileID[:2]+"/"+fileID+".enc"` to match `blobPath()` shard logic
  - Fixed `DeleteBlobByPath` in `server/storage/blobs.go` to prepend `bs.rootDir` so cleanup uses absolute paths
- Fix 2: Dead code removal (commit: 71e7720)
  - Removed `blobPath := h.store.WriteBlob` / `_ = blobPath` from `uploadFile` in `server/handlers/files.go`

## Blockers

- None

## Notes for Next Iteration

- Phase 2: Client-Side Crypto Layer
- Phase 3: Web App Frontend
