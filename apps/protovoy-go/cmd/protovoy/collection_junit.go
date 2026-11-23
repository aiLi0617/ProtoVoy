package main

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
)

type junitReport struct {
	file     *os.File
	encoder  *xml.Encoder
	suite    xml.StartElement
	recorded int
}

func openJUnitReport(path string) (*junitReport, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, errors.New("could not create a new JUnit report file")
	}
	report := &junitReport{
		file: file, encoder: xml.NewEncoder(file),
		suite: xml.StartElement{Name: xml.Name{Local: "testsuite"}, Attr: []xml.Attr{
			{Name: xml.Name{Local: "name"}, Value: "ProtoVoy collection"},
		}},
	}
	if _, err := io.WriteString(file, xml.Header); err != nil {
		file.Close()
		return nil, errors.New("could not write JUnit report")
	}
	if err := report.encoder.EncodeToken(report.suite); err != nil {
		file.Close()
		return nil, errors.New("could not write JUnit report")
	}
	return report, nil
}

func (report *junitReport) record(item collectionResult) error {
	name := fmt.Sprintf("%d. %s", item.RequestIndex, item.Name)
	if item.Iteration != 0 {
		name = fmt.Sprintf("[iteration %d] %s", item.Iteration, name)
	}
	type failure struct {
		Message string `xml:"message,attr"`
		Text    string `xml:",chardata"`
	}
	test := struct {
		XMLName xml.Name `xml:"testcase"`
		Name    string   `xml:"name,attr"`
		Class   string   `xml:"classname,attr"`
		Time    string   `xml:"time,attr"`
		Failure *failure `xml:"failure,omitempty"`
	}{
		Name: name, Class: item.Protocol, Time: strconv.FormatFloat(item.DurationMS/1000, 'f', 3, 64),
	}
	if !item.Passed {
		message := item.Error
		if message == "" {
			message = fmt.Sprintf("HTTP status %d", item.Status)
		}
		test.Failure = &failure{Message: message, Text: message}
	}
	if err := report.encoder.Encode(test); err != nil {
		return errors.New("could not write JUnit test result")
	}
	report.recorded++
	return nil
}

func (report *junitReport) finish(incomplete bool, skipped int) error {
	var err error
	if incomplete {
		test := struct {
			XMLName xml.Name `xml:"testcase"`
			Name    string   `xml:"name,attr"`
			Error   string   `xml:"error"`
		}{Name: "Collection execution", Error: "Collection run ended before all results were recorded"}
		err = report.encoder.Encode(test)
	} else if skipped > 0 {
		test := struct {
			XMLName xml.Name `xml:"testcase"`
			Name    string   `xml:"name,attr"`
			Skipped struct {
				Message string `xml:"message,attr"`
			} `xml:"skipped"`
		}{Name: "Remaining collection requests"}
		test.Skipped.Message = fmt.Sprintf("%d request(s) skipped after the first failure", skipped)
		err = report.encoder.Encode(test)
	}
	if err == nil {
		err = report.encoder.EncodeToken(report.suite.End())
	}
	if err == nil {
		err = report.encoder.Close()
	}
	if errors.Join(err, report.file.Close()) != nil {
		return errors.New("could not finish JUnit report")
	}
	return nil
}
