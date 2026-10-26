package system

import "github.com/spf13/viper"

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
