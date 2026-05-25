package acp

// AdapterConfig describes how to spawn an ACP-compatible agent subprocess.
type AdapterConfig struct {
	Name    string
	Command []string
	Env     []string
	Meta    map[string]any
}

// CursorAdapter returns the default configuration for spawning the Cursor
// ACP adapter. Requires npx (Node.js) to be available.
func CursorAdapter() AdapterConfig {
	return AdapterConfig{
		Name:    "cursor",
		Command: []string{"npx", "-y", "cursor-agent-acp"},
	}
}

// PiAdapter returns the default configuration for spawning the Pi coding
// agent via its ACP adapter. Requires npx (Node.js) to be available.
func PiAdapter() AdapterConfig {
	return AdapterConfig{
		Name:    "pi",
		Command: []string{"npx", "-y", "pi-acp"},
	}
}

// PiDirectAdapter returns a configuration that launches Pi directly in
// RPC mode without the ACP adapter shim.
func PiDirectAdapter() AdapterConfig {
	return AdapterConfig{
		Name:    "pi-direct",
		Command: []string{"pi", "--mode", "rpc"},
	}
}

// CustomAdapter creates a configuration for any ACP-compliant agent
// that communicates over stdio.
func CustomAdapter(name string, command []string, env []string) AdapterConfig {
	return AdapterConfig{
		Name:    name,
		Command: command,
		Env:     env,
	}
}
