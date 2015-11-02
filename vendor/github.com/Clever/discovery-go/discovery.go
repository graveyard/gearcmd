package discovery

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	kv "gopkg.in/Clever/kayvee-go.v2"
)

// m is a convenience type for using kv.
type m map[string]interface{}

const (
	templateVar = "SERVICE_%s_%s_%%s"
)

func getVar(envVar string) (string, error) {
	envVar = strings.ToUpper(envVar)
	envVar = strings.Replace(envVar, "-", "_", -1)
	val := os.Getenv(envVar)
	if val == "" {
		return "", errors.New(kv.FormatLog("discovery-go", kv.Error, "missing env var", m{
			"var": envVar,
		}))
	}
	return val, nil
}

// URL finds the specified URL for a service based off of the service's name and which
// interface you are accessing. Values are found in environment variables fitting the scheme:
// SERVICE_{SERVICE NAME}_{INTERFACE NAME}_{PROTO,HOST,PORT}.
func URL(service, name string) (string, error) {
	proto, err := Proto(service, name)
	if err != nil {
		return "", err
	}
	host, err := Host(service, name)
	if err != nil {
		return "", err
	}
	port, err := Port(service, name)
	if err != nil {
		return "", err
	}

	u := url.URL{
		Scheme: proto,
		Host:   fmt.Sprintf("%s:%s", host, port),
	}
	return u.String(), nil
}

// Proto finds the specified protocol for a service based off of the service's name and which
// interface you are accessing. Values are found in environment variables fitting the scheme:
// SERVICE_{SERVICE NAME}_{INTERFACE NAME}_PROTO.
func Proto(service, name string) (string, error) {
	template := fmt.Sprintf(templateVar, service, name)
	return getVar(fmt.Sprintf(template, "PROTO"))
}

// Host finds the specified host for a service based off of the service's name and which
// interface you are accessing. Values are found in environment variables fitting the scheme:
// SERVICE_{SERVICE NAME}_{INTERFACE NAME}_HOST.
func Host(service, name string) (string, error) {
	template := fmt.Sprintf(templateVar, service, name)
	return getVar(fmt.Sprintf(template, "HOST"))
}

// Port finds the specified port for a service based off of the service's name and which
// interface you are accessing. Values are found in environment variables fitting the scheme:
// SERVICE_{SERVICE NAME}_{INTERFACE NAME}_PORT.
func Port(service, name string) (string, error) {
	template := fmt.Sprintf(templateVar, service, name)
	return getVar(fmt.Sprintf(template, "PORT"))
}
