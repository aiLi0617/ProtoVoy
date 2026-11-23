package execution

import (
	"encoding/json"
	"errors"
)

const RedisBridgePath = "/api/redis"
const RedisPasswordEnv = "PROTOVOY_REDIS_PASSWORD"
const RedisPasswordRefEnv = "PROTOVOY_REDIS_PASSWORD_REF"
const redisMessageLimit = 1 << 20
const RedisJSONResponseLimit = 16 << 20

// RedisOptions are local connection settings, separate from the browser payload.
type RedisOptions struct {
	Protocol    int
	Username    string
	Password    string
	PasswordRef string
	Database    string
	TLS         bool
	ServerName  string
	CAPEM       string
}

type RedisPayload struct {
	TimeoutMS           int        `json:"timeoutMs,omitempty"`
	ExpectedTarget      string     `json:"expectedTarget"`
	ExpectedPasswordRef string     `json:"expectedPasswordRef,omitempty"`
	ExpectedUsername    string     `json:"expectedUsername,omitempty"`
	ExpectedDatabase    string     `json:"expectedDatabase,omitempty"`
	ExpectedServerName  string     `json:"expectedServerName,omitempty"`
	Commands            [][]string `json:"commands"`
	Protocol            int        `json:"protocol,omitempty"`
}

// ParseRedisCommands decodes the workbench JSON without treating null as an
// empty string. Resolve variables in the decoded arguments, not the JSON.
func ParseRedisCommands(input string) ([][]string, error) {
	var raw [][]*string
	if len(input) > redisMessageLimit || json.Unmarshal([]byte(input), &raw) != nil || len(raw) == 0 || len(raw) > 1024 {
		return nil, errors.New(`enter Redis commands as JSON, for example [["PING"]]`)
	}
	commands := make([][]string, len(raw))
	for i, command := range raw {
		if len(command) == 0 || len(command) > 1024 {
			return nil, errors.New("Redis requires 1 to 1024 arguments per command")
		}
		commands[i] = make([]string, len(command))
		for j, arg := range command {
			if arg == nil {
				return nil, errors.New("Redis command arguments must be strings")
			}
			commands[i][j] = *arg
		}
	}
	return commands, nil
}

func (payload *RedisPayload) ResolveVariables(scopes ...map[string]string) error {
	resolver := newVariableResolver(scopes)
	payload.ExpectedTarget = resolver.text(payload.ExpectedTarget)
	for _, command := range payload.Commands {
		for i, argument := range command {
			command[i] = resolver.text(argument)
		}
	}
	return resolver.err
}

// RedisValue preserves RESP reply types without forcing map keys or doubles
// into JSON's string-keyed, finite-number model.
type RedisValue struct {
	Kind       string       `json:"kind"`
	Text       string       `json:"text,omitempty"`
	Data       []byte       `json:"data,omitempty"`
	Integer    int64        `json:"integer,omitempty"`
	Boolean    *bool        `json:"boolean,omitempty"`
	Format     string       `json:"format,omitempty"`
	Items      []RedisValue `json:"items,omitempty"`
	Entries    []RedisPair  `json:"entries,omitempty"`
	Attributes []RedisPair  `json:"attributes,omitempty"`
}

type RedisPair struct {
	Key   RedisValue `json:"key"`
	Value RedisValue `json:"value"`
}
