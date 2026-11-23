package execution

const SocketResponseLimit = 512 << 10

// SocketPayload is shared by native execution and the browser API client.
// The server supplies the actual target after checking ExpectedTarget.
type SocketPayload struct {
	Network        string `json:"network"`
	ExpectedTarget string `json:"expectedTarget,omitempty"`
	Data           string `json:"data"`
	Encoding       string `json:"encoding,omitempty"`
	Framing        string `json:"framing,omitempty"`
	Delimiter      string `json:"delimiter,omitempty"`
	FixedLength    int    `json:"fixedLength,omitempty"`
	TLS            bool   `json:"tls,omitempty"`
	ServerName     string `json:"serverName,omitempty"`
	CAPEM          string `json:"caPem,omitempty"`
	SendCount      int    `json:"sendCount,omitempty"`
	IntervalMS     int    `json:"intervalMs,omitempty"`
	TimeoutMS      int    `json:"timeoutMs,omitempty"`
}

func (payload SocketPayload) ValidSendOptions() bool {
	return payload.SendCount >= 0 && payload.SendCount <= 10_000 && payload.IntervalMS >= 0 && payload.IntervalMS <= 300_000
}

type SocketResponse struct {
	Data string `json:"dataBase64"`
}

func (payload SocketPayload) ResolveVariables(scopes ...map[string]string) (SocketPayload, error) {
	resolver := newVariableResolver(scopes)
	payload.ExpectedTarget = resolver.text(payload.ExpectedTarget)
	payload.ServerName = resolver.text(payload.ServerName)
	payload.Delimiter = resolver.text(payload.Delimiter)
	if payload.Encoding == "" || payload.Encoding == "text" {
		payload.Data = resolver.text(payload.Data)
	}
	return payload, resolver.err
}
