package config

import (
	"github.com/facebookgo/clock"
	"gopkg.in/Clever/kayvee-go.v6/logger"
)

var (
	Clock clock.Clock = clock.New()
)

func init() {
	data, err := Asset("kvconfig.yml")
	if err != nil {
		panic(err)
	}
	logger.SetGlobalRoutingFromBytes(data)
}
