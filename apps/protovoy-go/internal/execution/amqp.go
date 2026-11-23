package execution

const AMQPBridgePath = "/api/amqp"
const AMQPPasswordEnv = "PROTOVOY_AMQP_PASSWORD"
const AMQPPasswordRefEnv = "PROTOVOY_AMQP_PASSWORD_REF"

type AMQPOptions struct {
	TimeoutMS                                   int    `json:"-"`
	PasswordRef                                 string `json:"passwordRef,omitempty"`
	Mode, Username, Password, ServerName, CAPEM string
	Exchange, ExchangeType, RoutingKey, Queue   string
	Payload, Encoding, ContentType              string
	Declare, Durable, AutoAck                   bool
	ReceiveLimit                                int
}

func (options *AMQPOptions) ResolveVariables(target string, scopes ...map[string]string) (string, error) {
	resolve := newVariableResolver(scopes)
	target = resolve.text(target)
	for _, field := range []*string{&options.Exchange, &options.RoutingKey, &options.Queue, &options.Payload,
		&options.ContentType, &options.Username, &options.ServerName} {
		*field = resolve.text(*field)
	}
	return target, resolve.err
}

type AMQPBridgeRequest struct {
	TimeoutMS           int    `json:"timeoutMs,omitempty"`
	ExpectedTarget      string `json:"expectedTarget,omitempty"`
	ExpectedPasswordRef string `json:"expectedPasswordRef,omitempty"`
	Mode                string `json:"mode"`
	Exchange            string `json:"exchange,omitempty"`
	ExchangeType        string `json:"exchangeType,omitempty"`
	RoutingKey          string `json:"routingKey,omitempty"`
	Queue               string `json:"queue,omitempty"`
	Payload             string `json:"payload,omitempty"`
	Encoding            string `json:"encoding,omitempty"`
	ContentType         string `json:"contentType,omitempty"`
	Declare             *bool  `json:"declare,omitempty"`
	Durable             bool   `json:"durable,omitempty"`
	AutoAck             bool   `json:"autoAck,omitempty"`
	ReceiveLimit        int    `json:"receiveLimit,omitempty"`
}
