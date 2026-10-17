package system

import (
	"fmt"

	"github.com/spf13/viper"
)

/*
Paper configures where `make run` keeps its paper-trade records. Run never
writes storage, so every record is appended to the local file RecordsPath;
`symm paper` uploads that file under the paper/ prefix. There is no default
in code: the configuration must state it (cmd/cfg/config.yml).
*/
type Paper struct {
	RecordsPath string
}

func NewPaper() *Paper {
	return &Paper{RecordsPath: viper.GetString("paper.records_path")}
}

/*
Validate rejects a paper configuration without a records path.
*/
func (paper *Paper) Validate() error {
	if paper == nil {
		return fmt.Errorf("paper configuration is missing")
	}

	if paper.RecordsPath == "" {
		return fmt.Errorf("paper.records_path is not configured")
	}

	return nil
}
