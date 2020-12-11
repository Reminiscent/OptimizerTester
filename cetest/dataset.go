package cetest

import (
	"fmt"
	"github.com/qw4990/OptimizerTester/tidb"
	"math/rand"
)

// Dataset ...
type Dataset interface {
	// Name returns the name of the dataset
	Name() string

	// Init ...
	Init(instances []tidb.Instance, queryTypes []QueryType) error

	// GenEstResults ...
	GenEstResults(n int, ins tidb.Instance, qt QueryType) ([]EstResult, error)
}

type tableVals struct {
	tbs             []string     // table names
	cols            [][]string   // table columns' names
	orderedDistVals [][][]string // ordered distinct values
	valActRows      [][][]int    // actual row count
}

func newTableVals(ins tidb.Instance, tbs []string, cols [][]string) (*tableVals, error) {
	tv := &tableVals{
		tbs:             tbs,
		cols:            cols,
		orderedDistVals: newStrArray(cols),
		valActRows:      newIntArray(cols),
	}
	return tv, fillTableVals(ins, tv)
}

func newStrArray(cols [][]string) [][][]string {
	vals := make([][][]string, len(cols))
	for i := range cols {
		vals[i] = make([][]string, len(cols[i]))
	}
	return vals
}

func newIntArray(cols [][]string) [][][]int {
	vals := make([][][]int, len(cols))
	for i := range cols {
		vals[i] = make([][]int, len(cols[i]))
	}
	return vals
}

func fillTableVals(ins tidb.Instance, tv *tableVals) error {
	for i, tb := range tv.tbs {
		for j, col := range tv.cols[i] {
			q := fmt.Sprintf("SELECT %v, COUNT(*) FROM %v GROUP BY %v ORDER BY COUNT(*)", col, tb, col)
			rows, err := ins.Query(q)
			if err != nil {
				return err
			}
			for rows.Next() {
				var val string
				var cnt int
				if err := rows.Scan(&val, &cnt); err != nil {
					return err
				}
				tv.orderedDistVals[i][j] = append(tv.orderedDistVals[i][j], val)
				tv.valActRows[i][j] = append(tv.valActRows[i][j], cnt)
			}
			if err := rows.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (tv *tableVals) randPointCond(tbIdx, colIdx int) (cond string, actRows int) {
	x := rand.Intn(len(tv.orderedDistVals[tbIdx][colIdx]))
	cond = fmt.Sprintf("%v=%v", tv.cols[tbIdx][colIdx], tv.orderedDistVals[tbIdx][colIdx][x])
	actRows = tv.valActRows[tbIdx][colIdx][x]
	return
}

func (tv *tableVals) randRangeCond(tbIdx, colIdx int) (cond string, actRows int) {
	l := rand.Intn(len(tv.orderedDistVals[tbIdx][colIdx]))
	r := rand.Intn(len(tv.orderedDistVals[tbIdx][colIdx])-1) + l
	cond = fmt.Sprintf("%v>=%v AND %v<=%v", tv.cols[tbIdx][colIdx],
		tv.orderedDistVals[tbIdx][colIdx][l], tv.cols[tbIdx][colIdx], tv.orderedDistVals[tbIdx][colIdx][r])
	actRows = 0
	for i := l; i <= r; i++ {
		actRows += tv.valActRows[tbIdx][colIdx][i]
	}
	return
}

func (tv *tableVals) randMCVPointCond(tbIdx, colIdx, percent int) (cond string, actRows int) {
	n := len(tv.orderedDistVals[tbIdx][colIdx])
	width := n * percent / 100
	x := rand.Intn(width) + (n - width)
	cond = fmt.Sprintf("%v=%v", tv.cols[tbIdx][colIdx], tv.orderedDistVals[tbIdx][colIdx][x])
	actRows = tv.valActRows[tbIdx][colIdx][x]
	return
}
