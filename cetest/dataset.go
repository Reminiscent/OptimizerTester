package cetest

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/qw4990/OptimizerTester/tidb"
)

// Dataset ...
type Dataset interface {
	// Name returns the name of the dataset
	Name() string

	// GenCases ...
	GenCases(n int, qt QueryType) (queries []string, err error)
}

type baseDataset struct {
	opt DatasetOpt
	ins tidb.Instance

	numTbs      int
	numCols     []int
	tbs         []string
	cols        [][]string
	used        [][]bool     // `used[i][j] = false` means we will not use the value of (table[i], column[j])
	orderedVals [][][]string // [tbIdx][colIdx][]string{ordered values}
	mcv         [][][]string // mcv[i][j] means the most common values in (table[i], column[j]) from the dataset
	lcv         [][][]string // least common values
	percent     int
}

func newBaseDataset(opt DatasetOpt, ins tidb.Instance, tbs []string, cols [][]string, used [][]bool) (baseDataset, error) {
	numTbs := len(tbs)
	numCols := make([]int, numTbs)
	for idx, col := range cols {
		numCols[idx] = len(col)
	}
	return baseDataset{
		opt:     opt,
		ins:     ins,
		numTbs:  numTbs,
		numCols: numCols,
		tbs:     tbs,
		cols:    cols,
		used:    used,
		percent: 10, // most/least 10% common values
	}, nil
}

func (ds *baseDataset) init() error {
	// switch database
	if err := ds.ins.Exec(fmt.Sprintf("USE %v", ds.opt.DB)); err != nil {
		return err
	}

	// analyze tables
	for tbIdx, tb := range ds.tbs {
		used := false
		for _, flag := range ds.used[tbIdx] {
			if flag {
				used = true
				break
			}
		}
		if !used {
			continue
		}
		if err := ds.ins.Exec(fmt.Sprintf("ANALYZE TABLE %v", tb)); err != nil {
			return err
		}
	}

	// init ordered values
	ds.orderedVals = ds.valArray()
	for i, tb := range ds.tbs {
		for j, col := range ds.cols[i] {
			if !ds.used[i][j] {
				continue
			}
			sql := fmt.Sprintf("SELECT %v FROM %v ORDER BY %v", col, tb, col)
			begin := time.Now()
			rows, err := ds.ins.Query(sql)
			if err != nil {
				return err
			}
			fmt.Printf("[%v-%v] %v cost %v\n", ds.opt.Label, ds.ins.Opt().Label, sql, time.Since(begin))
			for rows.Next() {
				var val interface{}
				if err := rows.Scan(&val); err != nil {
					return err
				}
				ds.orderedVals[i][j] = append(ds.orderedVals[i][j], val.(string))
			}
			rows.Close()
		}
	}

	// init mcv and lcv
	ds.mcv = ds.valArray()
	ds.lcv = ds.valArray()
	for i, tb := range ds.tbs {
		row, err := ds.ins.Query(fmt.Sprintf("SELECT COUNT(*) FROM %v", tb))
		if err != nil {
			return err
		}
		var total int
		if err := row.Scan(&total); err != nil {
			return err
		}
		row.Close()
		limit := total * ds.percent / 100

		for j, col := range ds.cols[i] {
			if !ds.used[i][j] {
				continue
			}
			for _, order := range []string{"DESC", "ASC"} {
				rows, err := ds.ins.Query(fmt.Sprintf("SELECT %v FROM %v GROUP BY %v ORDER BY COUNT(*) %v LIMIT %v", col, tb, col, order, limit))
				if err != nil {
					return err
				}
				for rows.Next() {
					var val interface{}
					if err := rows.Scan(&val); err != nil {
						return err
					}
					if order == "DESC" {
						ds.mcv[i][j] = append(ds.mcv[i][j], val.(string))
					} else {
						ds.lcv[i][j] = append(ds.lcv[i][j], val.(string))
					}
				}
				rows.Close()
			}
		}
	}
	return nil
}

func (ds *baseDataset) valArray() [][][]string {
	xs := make([][][]string, ds.numTbs)
	for i := range xs {
		xs[i] = make([][]string, ds.numCols[i])
	}
	return xs
}

func (ds *baseDataset) randPointColCond(tbIdx, colIdx int) string {
	val := ds.orderedVals[tbIdx][colIdx][rand.Intn(len(ds.orderedVals[tbIdx][colIdx]))]
	return fmt.Sprintf("%v = %v", ds.cols[tbIdx][colIdx], val)
}

func (ds *baseDataset) randRangeColCond(tbIdx, colIdx int) string {
	val1Idx := rand.Intn(len(ds.orderedVals[tbIdx][colIdx]))
	val2Idx := rand.Intn(len(ds.orderedVals[tbIdx][colIdx])-val1Idx) + val1Idx
	return fmt.Sprintf("%v>=%v AND %v<=%v", ds.cols[tbIdx][colIdx], ds.orderedVals[tbIdx][colIdx][val1Idx], ds.cols[tbIdx][colIdx], ds.orderedVals[tbIdx][colIdx][val2Idx])
}