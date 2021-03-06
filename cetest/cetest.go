package cetest

import (
	"bytes"
	"database/sql"
	"fmt"
	"io/ioutil"
	"path"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/pingcap/errors"
	"github.com/qw4990/OptimizerTester/tidb"
)

type DatasetOpt struct {
	Name  string `toml:"name"`
	DB    string `toml:"db"`
	Label string `toml:"label"`
}

type Option struct {
	QueryTypes []QueryType   `toml:"query-types"`
	Datasets   []DatasetOpt  `toml:"datasets"`
	Instances  []tidb.Option `toml:"instances"`
	ReportDir  string        `toml:"report-dir"`
	N          int           `toml:"n"`
}

// DecodeOption decodes option content.
func DecodeOption(content string) (Option, error) {
	var opt Option
	if _, err := toml.Decode(content, &opt); err != nil {
		return Option{}, errors.Trace(err)
	}
	for _, ds := range opt.Datasets {
		if _, ok := datasetMap[strings.ToLower(ds.Name)]; !ok {
			return Option{}, fmt.Errorf("unknown dateset=%v", ds.Name)
		}
	}
	return opt, nil
}

// QueryType ...
type QueryType int

const (
	QTSingleColPointQuery            QueryType = 0 // where c = ?; where c in (?, ... ?)
	QTSingleColRangeQuery            QueryType = 1 // where c >= ?; where c > ? and c < ?
	QTMultiColsPointQuery            QueryType = 2 // where c1 = ? and c2 = ?
	QTMultiColsRangeQueryFixedPrefix QueryType = 3 // where c1 = ? and c2 > ?
	QTMultiColsRangeQuery            QueryType = 4 // where c1 > ? and c2 > ?
	QTJoinEQ                         QueryType = 5 // where t1.c = t2.c
	QTJoinNonEQ                      QueryType = 6 // where t1.c > t2.c
	QTGroup                          QueryType = 7 // group by c
)

var (
	qtNameMap = map[QueryType]string{
		QTSingleColPointQuery:            "single-col-point-query",
		QTSingleColRangeQuery:            "single-col-range-query",
		QTMultiColsPointQuery:            "multi-cols-point-query",
		QTMultiColsRangeQueryFixedPrefix: "multi-cols-range-query-fixed-prefix",
		QTMultiColsRangeQuery:            "multi-cols-range-query",
		QTJoinEQ:                         "join-eq",
		QTJoinNonEQ:                      "join-non-eq",
		QTGroup:                          "group",
	}
)

func (qt QueryType) String() string {
	return qtNameMap[qt]
}

func (qt *QueryType) UnmarshalText(text []byte) error {
	for k, v := range qtNameMap {
		if v == string(text) {
			*qt = k
			return nil
		}
	}
	return errors.Errorf("unknown query-type=%v", string(text))
}

// Dataset ...
type Dataset interface {
	// Name returns the name of the dataset
	Name() string

	// GenCases ...
	GenCases(n int, qt QueryType) (queries []string, err error)
}

var datasetMap = map[string]Dataset{ // read-only
	"zipx": new(datasetZipFX),
	"imdb": new(datasetIMDB),
	"tpcc": new(datasetTPCC),
	"mock": new(datasetMock),
}

func RunCETestWithConfig(confPath string) error {
	confContent, err := ioutil.ReadFile(confPath)
	if err != nil {
		return errors.Trace(err)
	}
	opt, err := DecodeOption(string(confContent))
	if err != nil {
		return err
	}

	instances, err := tidb.ConnectToInstances(opt.Instances)
	if err != nil {
		return errors.Trace(err)
	}

	collector := NewEstResultCollector(len(instances), len(opt.Datasets), len(opt.QueryTypes))
	var wg sync.WaitGroup
	insErrs := make([]error, len(instances))
	for insIdx := range instances {
		wg.Add(1)
		go func(insIdx int) {
			ins := instances[insIdx]
			for dsIdx, dataset := range opt.Datasets {
				ds := datasetMap[dataset.Name]
				if err := ins.Exec("use " + dataset.DB); err != nil {
					insErrs[insIdx] = err
					return
				}
				for qtIdx, qt := range opt.QueryTypes {
					qs, err := ds.GenCases(opt.N, qt)
					if err != nil {
						insErrs[insIdx] = err
						return
					}
					for _, q := range qs {
						estResult, err := runOneEstCase(ins, q)
						if err != nil {
							insErrs[insIdx] = err
							return
						}
						collector.AddEstResult(insIdx, dsIdx, qtIdx, estResult)
					}
				}
			}
		}(insIdx)
	}
	wg.Wait()

	for _, err := range insErrs {
		if err != nil {
			return err
		}
	}

	return GenReport(opt, collector)
}

func runOneEstCase(ins tidb.Instance, query string) (EstResult, error) {
	rows, err := ins.Query(query)
	if err != nil {
		return EstResult{}, errors.Trace(err)
	}
	defer rows.Close()
	return parseEstResult(rows)
}

func parseEstResult(rows *sql.Rows) (EstResult, error) {
	// TODO
	return EstResult{}, nil
}

// GenReport generates a report with MarkDown format.
func GenReport(opt Option, collector EstResultCollector) error {
	mdContent := bytes.Buffer{}
	for qtIdx, qt := range opt.QueryTypes {
		picPath, err := DrawBiasBoxPlotGroupByQueryType(opt, collector, qtIdx)
		if err != nil {
			return err
		}
		if _, err := mdContent.WriteString(fmt.Sprintf("%v: ![pic](%v)\n", qt, picPath)); err != nil {
			return errors.Trace(err)
		}
	}
	return ioutil.WriteFile(path.Join(opt.ReportDir, "report.md"), mdContent.Bytes(), 0666)
}
