package execution

const MQTTBridgePath = "/api/mqtt"
const MQTTPasswordEnv = "PROTOVOY_MQTT_PASSWORD"
const MQTTPasswordRefEnv = "PROTOVOY_MQTT_PASSWORD_REF"

type MQTTOptions struct {
	TimeoutMS                                                    int    `json:"-"`
	PasswordRef                                                  string `json:"passwordRef,omitempty"`
	Mode, ClientID, Username, Password, Topic, Payload, Encoding string
	CAPEM, ServerName                                            string
	CleanSession, Retain                                         bool
	KeepAlive, QoS, ReceiveLimit                                 int
}

func (options *MQTTOptions) ResolveVariables(target string, scopes ...map[string]string) (string, error) {
	resolve := newVariableResolver(scopes)
	target = resolve.text(target)
	options.Topic = resolve.text(options.Topic)
	options.Payload = resolve.text(options.Payload)
	options.ClientID = resolve.text(options.ClientID)
	options.Username = resolve.text(options.Username)
	options.ServerName = resolve.text(options.ServerName)
	return target, resolve.err
}

type MQTTBridgeRequest struct {
	TimeoutMS           int    `json:"timeoutMs,omitempty"`
	ExpectedTarget      string `json:"expectedTarget,omitempty"`
	ExpectedPasswordRef string `json:"expectedPasswordRef,omitempty"`
	Mode                string `json:"mode"`
	Topic               string `json:"topic"`
	Payload             string `json:"payload,omitempty"`
	Encoding            string `json:"encoding,omitempty"`
	QoS                 int    `json:"qos"`
	Retain              bool   `json:"retain,omitempty"`
	ReceiveLimit        int    `json:"receiveLimit,omitempty"`
}
