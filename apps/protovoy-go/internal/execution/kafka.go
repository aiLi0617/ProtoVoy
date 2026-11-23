package execution

const KafkaBridgePath = "/api/kafka"
const KafkaPasswordEnv = "PROTOVOY_KAFKA_PASSWORD"
const KafkaPasswordRefEnv = "PROTOVOY_KAFKA_PASSWORD_REF"

type KafkaOperation struct {
	Mode         string `json:"mode,omitempty"`
	Topic        string `json:"topic"`
	Key          string `json:"key,omitempty"`
	Payload      string `json:"payload,omitempty"`
	Encoding     string `json:"encoding,omitempty"`
	Partition    *int32 `json:"partition,omitempty"`
	GroupID      string `json:"groupId,omitempty"`
	OffsetReset  string `json:"offsetReset,omitempty"`
	AutoCommit   bool   `json:"autoCommit,omitempty"`
	ReceiveLimit int    `json:"receiveLimit,omitempty"`
}

type KafkaOptions struct {
	TimeoutMS int `json:"-"`
	KafkaOperation
	KafkaTLSReferences
	SecurityProtocol, SASLMechanism string
	Username, Password              string
	PasswordRef                     string `json:"passwordRef,omitempty"`
	CAPEM, CertificatePEM, KeyPEM   string
}

type KafkaTLSReferences struct {
	CAPEMRef          string `json:"caPemRef,omitempty"`
	CertificatePEMRef string `json:"certificatePemRef,omitempty"`
	KeyPEMRef         string `json:"keyPemRef,omitempty"`
}

func (refs KafkaTLSReferences) Validate() bool {
	return ValidPasswordReference(refs.CAPEMRef) && ValidPasswordReference(refs.CertificatePEMRef) &&
		ValidPasswordReference(refs.KeyPEMRef) && (refs.CertificatePEMRef == "") == (refs.KeyPEMRef == "")
}

type KafkaBridgeRequest struct {
	TimeoutMS             int                `json:"timeoutMs,omitempty"`
	ExpectedTarget        string             `json:"expectedTarget,omitempty"`
	ExpectedPasswordRef   string             `json:"expectedPasswordRef,omitempty"`
	ExpectedTLSReferences KafkaTLSReferences `json:"expectedTlsReferences,omitzero"`
	KafkaOperation
}

func (options *KafkaOptions) ResolveVariables(target string, scopes ...map[string]string) (string, error) {
	resolver := newVariableResolver(scopes)
	target = resolver.text(target)
	for _, field := range []*string{&options.Topic, &options.Key, &options.Payload, &options.GroupID, &options.Username} {
		*field = resolver.text(*field)
	}
	return target, resolver.err
}
