# Local Observability

The node emits JSON logs to stderr at `DEBUG`, `INFO`, `WARN`, or `ERROR` (default `INFO`). Lifecycle, public network identity, public PeerID, roles, accepted height, StateHash, and bounded error summaries are useful operational context. Bearer tokens, private keys/seeds, canonical transaction bodies, Board bodies, and local secret-file contents are never log attributes.

`GET /v1/health` distinguishes liveness from readiness. A starting or running process can be live; readiness requires `RUNNING` and `SYNCED`.

`GET /metrics` uses Prometheus text format on exactly the configured API listener. Disabling metrics returns 404. A non-loopback listener still requires the API bearer token, and the same exact-origin CORS and concurrency limits apply. Metrics cover node/build information, readiness/uptime/sync, P2P connected and outbound peers, consensus/state height and validator activity, bounded canonical counts, Board index counts, object usage/quota/fetch results, recent transaction occupancy, state-sync size limit, and API requests/rejections.

Labels are restricted to the bounded network, protocol, software, and sync-status vocabularies. PeerID, IdentityID, TxID, ProposalID, ObjectID, ResearchID, ResourceID, PostID, paths, addresses, and error text are not labels. Metrics and logs are local operational inputs and cannot change StateHash/AppHash.
