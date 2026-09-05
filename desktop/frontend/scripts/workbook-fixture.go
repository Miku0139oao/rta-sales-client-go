//go:build ignore

// Browser-smoke helper. Generates/inspects supplied synthetic workbooks only; never connects to RTA.
package main

import (
	"encoding/json"
	"fmt"
	"github.com/Miku0139oao/rta-sales-client-go/desktop"
	"github.com/xuri/excelize/v2"
	"os"
)

func main() {
	if len(os.Args) > 1 {
		f, err := excelize.OpenFile(os.Args[1])
		if err != nil {
			panic(err)
		}
		defer f.Close()
		out := map[string][][]string{}
		for _, name := range f.GetSheetList() {
			rows, err := f.GetRows(name, excelize.Options{RawCellValue: true})
			if err != nil {
				panic(err)
			}
			out[name] = rows
		}
		if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
			panic(err)
		}
		return
	}
	var request desktop.AnalysisWorkbookRequest
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		panic(err)
	}
	data, err := (&desktop.App{}).BuildSalesAnalysisWorkbook(request)
	if err != nil {
		panic(err)
	}
	fmt.Print(data)
}
