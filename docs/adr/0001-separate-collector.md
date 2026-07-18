# ADR 0001: Separate collector process

Status: accepted.

The collector runs independently from Unreal. Feed connectivity, normalization,
recording, and replay must survive client restarts and must not compete with the
render thread. A local WebSocket is the bootstrap transport. This adds one
process and a transport boundary, but enables multiple clients, headless
recording, independent failure domains, and repeatable replay.

