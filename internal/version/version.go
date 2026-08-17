package version

// Version is stamped at build time via -ldflags "-X .../version.Version=...".
var Version = "1.0.0"

// ProtocolVersion is the wire protocol version. The client and server must
// agree on this value before a tunnel is established.
const ProtocolVersion = 1
