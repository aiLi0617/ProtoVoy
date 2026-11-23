package project

import "errors"

func (request Request) HasScripts() bool {
	return len(request.PreScripts) != 0 || len(request.PostScripts) != 0
}

func (request Request) ValidateScripts() error {
	if !request.HasScripts() {
		return nil
	}
	if request.CollectionHTTPProtocol() != "http" || request.HTTPSecretReference != "" || request.HTTPSecretHeader != "" {
		return errors.New("request scripts require HTTP without a credential binding")
	}
	total := 0
	for _, phase := range [][]string{request.PreScripts, request.PostScripts} {
		if len(phase) > 32 {
			return errors.New("a request script phase supports up to 32 scripts")
		}
		for _, source := range phase {
			total += len(source)
		}
	}
	if total > 128<<10 {
		return errors.New("request scripts exceed 128 KiB")
	}
	return nil
}
