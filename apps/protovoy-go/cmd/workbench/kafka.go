package main

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/vivaui/viva/app"
	"github.com/vivaui/viva/flex"
	"github.com/vivaui/viva/text"
	"github.com/vivaui/viva/ui"
	"protovoy.dev/protovoy/internal/execution"
	"protovoy.dev/protovoy/internal/project"
)

var (
	kafkaModeID        = tree.ID(page, "kafka-mode")
	kafkaBackID        = tree.ID(page, "kafka-back")
	kafkaSettingsID    = tree.ID(page, "kafka-settings")
	kafkaSwitchID      = tree.ID(page, "kafka-switch")
	kafkaTopicID       = tree.ID(page, "kafka-topic")
	kafkaKeyID         = tree.ID(page, "kafka-key")
	kafkaPartitionID   = tree.ID(page, "kafka-partition")
	kafkaEncodingID    = tree.ID(page, "kafka-encoding")
	kafkaGroupID       = tree.ID(page, "kafka-group")
	kafkaOffsetID      = tree.ID(page, "kafka-offset")
	kafkaLimitID       = tree.ID(page, "kafka-limit")
	kafkaCommitID      = tree.ID(page, "kafka-commit")
	kafkaSecurityID    = tree.ID(page, "kafka-security")
	kafkaSASLID        = tree.ID(page, "kafka-sasl")
	kafkaUserID        = tree.ID(page, "kafka-user")
	kafkaPasswordID    = tree.ID(page, "kafka-password")
	kafkaPasswordRefID = tree.ID(page, "kafka-password-ref")
	kafkaCARefID       = tree.ID(page, "kafka-ca-ref")
	kafkaCertRefID     = tree.ID(page, "kafka-cert-ref")
	kafkaKeyRefID      = tree.ID(page, "kafka-key-ref")
	kafkaTLSRefsID     = tree.ID(page, "kafka-tls-refs")
	kafkaCAID          = tree.ID(page, "kafka-ca")
	kafkaCertID        = tree.ID(page, "kafka-client-cert")
	kafkaPrivateKeyID  = tree.ID(page, "kafka-private-key")
	kafkaClearTLSID    = tree.ID(page, "kafka-clear-tls")
)

type kafkaTLSImport struct {
	key  bool
	data []byte
	err  error
}

type kafkaPanel struct {
	mode                                                  string
	settings, autoCommit, showTLSRefs                     bool
	topic, key, partition, encoding, group, offset, limit flex.Field
	security, sasl, username, password, passwordRef       flex.Field
	caRef, certRef, keyRef                                flex.Field
	ca                                                    requestEditor
	certificate, privateKey, tlsNotice                    string
	picking                                               bool
	imported                                              *kafkaTLSImport // protected by workbench.mu
	result                                                result
}

func defaultKafkaRequest() project.Request {
	return project.Request{Kafka: &project.KafkaSettings{Mode: "produce", Encoding: "text", OffsetReset: "latest", ReceiveLimit: "1"}}
}

func (w *workbench) kafkaDraft() project.Draft {
	p := &w.kafka
	return project.Draft{
		BrokerTimeoutMS: w.brokerTimeout.Text(),
		Kafka: project.KafkaSettings{
			Mode: p.mode, Topic: p.topic.Text(), Key: p.key.Text(), Partition: p.partition.Text(), Encoding: p.encoding.Text(),
			GroupID: p.group.Text(), OffsetReset: p.offset.Text(), ReceiveLimit: p.limit.Text(), AutoCommit: p.autoCommit,
			SecurityProtocol: p.security.Text(), SASLMechanism: p.sasl.Text(), Username: p.username.Text(),
			PasswordRef: p.passwordRef.Text(),
			KafkaTLSReferences: execution.KafkaTLSReferences{
				CAPEMRef: p.caRef.Text(), CertificatePEMRef: p.certRef.Text(), KeyPEMRef: p.keyRef.Text(),
			},
		},
		Name: w.requestName.Text(), Target: w.target.Text(), Body: w.requestBody.input.Editor.String(),
	}
}

func (w *workbench) applyKafkaDraft(s project.KafkaSettings) {
	p := &w.kafka
	p.mode, p.autoCommit, p.settings = s.Mode, s.AutoCommit, false
	p.showTLSRefs = false
	p.topic.SetText(s.Topic)
	p.key.SetText(s.Key)
	p.partition.SetText(s.Partition)
	p.encoding.SetText(s.Encoding)
	p.group.SetText(s.GroupID)
	p.offset.SetText(s.OffsetReset)
	p.limit.SetText(s.ReceiveLimit)
	p.security.SetText(s.SecurityProtocol)
	p.sasl.SetText(s.SASLMechanism)
	p.username.SetText(s.Username)
	p.password.SetText("")
	p.passwordRef.SetText(s.PasswordRef)
	p.caRef.SetText(s.CAPEMRef)
	p.certRef.SetText(s.CertificatePEMRef)
	p.keyRef.SetText(s.KeyPEMRef)
	p.ca.input.Editor = text.NewEditor("")
	p.certificate, p.privateKey, p.tlsNotice = "", "", ""
	p.result = result{}
}

func (w *workbench) drawKafka(f *flex.Frame, canvas *ui.Canvas) {
	p := &w.kafka
	direct := localTarget || w.useAgent()
	if p.settings {
		if !p.picking && f.Button(kafkaBackID, "Back to Kafka request").Clicked() {
			p.settings = false
		}
		f.Field(&p.passwordRef, kafkaPasswordRefID, "Kafka password reference", "Optional password reference")
		if !p.picking && f.Button(kafkaTLSRefsID, "TLS references").Clicked() {
			p.showTLSRefs = !p.showTLSRefs
		}
		if p.showTLSRefs {
			f.Field(&p.caRef, kafkaCARefID, "Kafka CA reference", "Optional CA reference")
			f.Field(&p.certRef, kafkaCertRefID, "Kafka client certificate reference", "Optional client certificate reference")
			f.Field(&p.keyRef, kafkaKeyRefID, "Kafka private key reference", "Optional private key reference")
			return
		}
		if !direct {
			return
		}
		f.Row().Gap(8).Children(func() {
			f.Field(&p.security, kafkaSecurityID, "Kafka security protocol", "PLAINTEXT, SSL, or SASL_SSL").Grow(1)
			f.Field(&p.sasl, kafkaSASLID, "Kafka SASL mechanism", "PLAIN or SCRAM-SHA-256/512").Grow(1)
		})
		f.Row().Gap(8).Children(func() {
			f.Field(&p.username, kafkaUserID, "Kafka username", "Username").Grow(1)
			f.Field(&p.password, kafkaPasswordID, "Kafka password", "Session password").Grow(1)
		})
		p.ca.draw(f, canvas, kafkaCAID, "Kafka trusted CA", "Optional PEM CA certificates").Height(84)
		f.Row().Gap(8).Children(func() {
			if p.picking {
				f.Text("Opening PEM file…")
				return
			}
			if f.Button(kafkaCertID, "Load client certificate").Clicked() {
				w.importKafkaTLS(false)
				return
			}
			if f.Button(kafkaPrivateKeyID, "Load private key").Clicked() {
				w.importKafkaTLS(true)
				return
			}
			if (p.certificate != "" || p.privateKey != "") && f.Button(kafkaClearTLSID, "Clear client identity").Clicked() {
				p.certificate, p.privateKey, p.tlsNotice = "", "", ""
			}
		})
		if p.certificate != "" {
			f.Text("Client certificate loaded")
		}
		if p.privateKey != "" {
			f.Text("Private key loaded")
		}
		f.Wrap(p.tlsNotice)
		return
	}
	f.Row().Gap(8).Items(flex.Center).Children(func() {
		f.Text("Kafka request")
		if w.cancel != nil {
			return
		}
		if f.Button(kafkaBackID, "Back to requests").Clicked() && w.canLeaveDraft() {
			w.loadRequest(0, project.Request{})
		}
		if f.Button(kafkaSettingsID, "Connection settings").Clicked() {
			p.settings = true
		}
		next := "consume"
		if p.mode == "consume" {
			next = "produce"
		}
		if f.Button(kafkaSwitchID, "Switch to "+next).Clicked() {
			p.mode = next
		}
	})
	placeholder := "Server-configured Kafka brokers"
	if direct {
		placeholder = "localhost:9092"
	}
	f.Row().Gap(8).Children(func() {
		f.Field(&w.requestName, requestNameID, "Request name", "Request name").Grow(1)
		f.Field(&w.target, targetID, "Kafka broker addresses", placeholder).Grow(2)
	})
	f.Field(&p.topic, kafkaTopicID, "Kafka topic", "Topic")
	w.drawBrokerTimeout(f)
	if p.mode == "produce" {
		f.Row().Gap(8).Children(func() {
			f.Field(&p.key, kafkaKeyID, "Kafka record key", "Optional key").Grow(1)
			f.Field(&p.partition, kafkaPartitionID, "Kafka partition", "Optional partition").Grow(1)
			f.Field(&p.encoding, kafkaEncodingID, "Kafka payload encoding", "text or base64").Grow(1)
		})
		w.requestBody.draw(f, canvas, requestBodyID, "Kafka message payload", "Message payload").Height(76)
	} else {
		f.Row().Gap(8).Children(func() {
			f.Field(&p.group, kafkaGroupID, "Kafka consumer group", "Consumer group").Grow(1)
			f.Field(&p.offset, kafkaOffsetID, "Kafka offset reset", "latest, earliest, or error").Grow(1)
			f.Field(&p.limit, kafkaLimitID, "Kafka receive limit", "Receive limit").Grow(1)
		})
		drawAMQPOption(f, kafkaCommitID, &p.autoCommit, "Auto commit", "Manual commit")
	}
	f.Row().Gap(8).Children(func() {
		if w.cancel != nil {
			if f.Button(cancelID, "Cancel").Clicked() {
				w.cancel()
			}
			return
		}
		label := "Produce Kafka"
		if p.mode == "consume" {
			label = "Consume Kafka"
		}
		if w.useAgent() {
			label += " via Local Agent"
		}
		if f.Button(sendID, label).Clicked() {
			w.sendKafka()
		}
		if f.Button(saveRequestID, "Save request").Clicked() {
			w.saveDraft()
		}
		if f.Button(discardID, "Discard edits").Clicked() {
			w.discardDraft()
		}
	})
	f.Wrap(p.result.status)
	f.ScrollArea(bodyID).Follow(w.cancel != nil).Grow(1).MinHeight(responseMinHeight).Children(func() { f.Wrap(p.result.body) })
}

func (w *workbench) importKafkaTLS(key bool) {
	w.kafka.picking = true
	w.kafka.tlsNotice = ""
	chooseLocalFile(app.OpenFileOptions{
		Title:   "Load Kafka client identity PEM",
		Filters: []app.FileFilter{{Name: "PEM", Pattern: "*.pem"}, {Name: "Certificate", Pattern: "*.crt"}, {Name: "Private key", Pattern: "*.key"}},
	}, 1<<20, func(data []byte, err error) {
		w.mu.Lock()
		w.kafka.imported = &kafkaTLSImport{key: key, data: data, err: err}
		w.mu.Unlock()
		w.invalidator.Invalidate()
	})
}

func (p *kafkaPanel) finishTLSImport(imported kafkaTLSImport) {
	p.picking = false
	switch {
	case imported.err != nil:
		p.tlsNotice = "Could not load the selected PEM file within 1 MiB"
	case imported.data != nil && len(imported.data) == 0:
		p.tlsNotice = "Select a nonempty PEM file"
	case imported.data != nil && imported.key:
		p.privateKey = string(imported.data)
	case imported.data != nil:
		p.certificate = string(imported.data)
	}
}

func (w *workbench) sendKafka() {
	draft := w.kafkaDraft()
	timeoutMS, err := parseRequestTimeout(draft.BrokerTimeoutMS)
	if err != nil {
		w.kafka.result = result{status: err.Error(), done: true}
		return
	}
	settings := draft.Kafka
	operation, err := settings.Operation(draft.Body)
	if err != nil {
		w.kafka.result = result{status: err.Error(), done: true}
		return
	}
	options := execution.KafkaOptions{
		TimeoutMS:      timeoutMS,
		KafkaOperation: operation, PasswordRef: settings.PasswordRef, KafkaTLSReferences: settings.KafkaTLSReferences,
	}
	if localTarget || w.useAgent() {
		options.SecurityProtocol, options.SASLMechanism, options.Username = settings.SecurityProtocol, settings.SASLMechanism, settings.Username
		options.Password, options.CAPEM = w.kafka.password.Text(), w.kafka.ca.input.Editor.String()
		options.CertificatePEM, options.KeyPEM = w.kafka.certificate, w.kafka.privateKey
	}
	target, err := options.ResolveVariables(strings.TrimSpace(draft.Target), w.variableLayers()...)
	if err != nil {
		w.kafka.result = result{status: err.Error(), done: true}
		return
	}
	if target == "" && (localTarget || w.useAgent()) {
		w.kafka.result = result{status: "Enter Kafka broker addresses", done: true}
		return
	}
	run := runDirectKafka
	if w.useAgent() {
		run = w.agent.client.RunKafka
	}
	w.startBrokerRun("Kafka", &w.kafka.result, func(ctx context.Context, connected func(), message func(json.RawMessage)) error {
		return run(ctx, target, options, connected, message)
	})
}
