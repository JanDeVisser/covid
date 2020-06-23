/*
 * This file is part of Finn.
 *
 * Copyright (c) 2020 Jan de Visser <jan@finiandarcy.com>
 *
 * Finn is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * Finn is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with Finn.  If not, see <https://www.gnu.org/licenses/>.
 */

package app

import (
	"fmt"
	"github.com/JanDeVisser/grumble"
	"github.com/JanDeVisser/grumble/handler"
	"github.com/wcharczuk/go-chart"
	"github.com/wcharczuk/go-chart/drawing"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type DataPoint struct {
	Date        time.Time
	NewCount    int
	Count       int
	NewDeceased int
	Deceased    int
}

type CasesChartSeries struct {
	DataSeries *DataSeries
	Which      int
	ChartType  string
	Regression bool
	Window     []float64
	Data       []float64
	Current    int
	New        int
	WindowPtr  int
}

type DataSeries struct {
	ChartData    *CasesChartData
	Jurisdiction *Jurisdiction
	Color        drawing.Color
	First        time.Time
	Last         time.Time
	Current      *DataPoint
	Cases        int
	Deaths       int
	DataPoints   []*DataPoint

	ConfirmedData *CasesChartSeries
	DeceasedData  *CasesChartSeries
}

const (
	ChartTypeAbsolute   = "ABS"
	ChartTypeRelative   = "REL"
	ChartTypeDaily      = "DAILY"
	ChartTypeRollingAvg = "ROLLING"
	ChartTypeSuppress   = "SUPPRESS"
	ChartTypeMortality  = "MORTALITY"
)

const (
	ChartDataCases  = 0
	ChartDataDeaths = 1
)

type CasesChartData struct {
	Manager         *grumble.EntityManager
	Query           *grumble.Query
	Country         *Jurisdiction
	Jurisdictions   []grumble.Persistable
	Regions         []grumble.Persistable
	Exclude         []grumble.Persistable
	Aggregate       bool
	ChartTypeCases  string
	ChartTypeDeaths string
	Regression      bool

	Results     [][]grumble.Persistable
	Series      map[string]*DataSeries
	First       time.Time
	Last        time.Time
	Days        int
	Population  float64
	ChartSeries []chart.Series
}

var Colors = []drawing.Color{
	chart.ColorRed,
	chart.ColorGreen,
	chart.ColorBlue,
	chart.ColorOrange,
	chart.ColorYellow,
	chart.ColorCyan,
}

//var ccc = chart.Style{
//	Hidden:              false,
//	Padding:             chart.Box{},
//	ClassName:           "",
//	StrokeWidth:         0,
//	StrokeColor:         drawing.Color{},
//	StrokeDashArray:     nil,
//	DotColor:            drawing.Color{},
//	DotWidth:            0,
//	DotWidthProvider:    nil,
//	DotColorProvider:    nil,
//	FillColor:           drawing.Color{},
//	FontSize:            0,
//	FontColor:           drawing.Color{},
//	Font:                nil,
//	TextHorizontalAlign: 0,
//	TextVerticalAlign:   0,
//	TextWrap:            0,
//	TextLineSpacing:     0,
//	TextRotationDegrees: 0,
//}

func MakeDataSeries(data *CasesChartData, jurisdiction *Jurisdiction, color drawing.Color, first *Sample) (series *DataSeries) {
	name := "Global"
	if jurisdiction != nil {
		name = jurisdiction.Name
	}
	series = new(DataSeries)
	series.ChartData = data
	series.Jurisdiction = jurisdiction
	series.Color = color
	series.First = first.Date
	series.Current = nil
	series.DataPoints = make([]*DataPoint, 0)
	data.Series[name] = series

	series.ConfirmedData = MakeCasesChartSeries(series, ChartDataCases, series.ChartData.ChartTypeCases, series.ChartData.Regression)
	series.DeceasedData = MakeCasesChartSeries(series, ChartDataDeaths, series.ChartData.ChartTypeDeaths, series.ChartData.Regression)

	return
}

func (series *DataSeries) AppendSample(sample *Sample) {
	if series.Current == nil || series.Current.Date.Before(sample.Date) {
		if series.Current != nil {
			series.Current.NewCount = series.Current.Count - series.Cases
			series.Current.NewDeceased = series.Current.Deceased - series.Deaths
			series.Cases = series.Current.Count
			series.Deaths = series.Current.Deceased
		}
		series.Last = sample.Date
		series.Current = new(DataPoint)
		series.Current.Date = sample.Date
		series.DataPoints = append(series.DataPoints, series.Current)
	}
	series.Current.Count += sample.Confirmed
	series.Current.Deceased += sample.Deceased
}

func (data *CasesChartData) GetDataSeries(jurisdiction *Jurisdiction, first *Sample) (series *DataSeries) {
	name := "Global"
	if jurisdiction != nil {
		name = jurisdiction.Name
	}
	var ok bool
	series, ok = data.Series[name]
	if !ok {
		series = MakeDataSeries(data, jurisdiction, Colors[len(data.Series)%len(Colors)], first)
		data.Series[name] = series
	}
	return
}

func MakeCasesChartData(req *http.Request) (ret *CasesChartData, err error) {
	ret = new(CasesChartData)
	if ret.Manager, err = grumble.MakeEntityManager(); err != nil {
		return
	}
	ret.Jurisdictions = make([]grumble.Persistable, 0)
	if countries := req.FormValue("country"); countries != "" {
		for _, country := range strings.Split(countries, ",") {
			if len(ret.Jurisdictions) == 6 {
				break
			}
			j := GetJurisdiction(country)
			if j != nil {
				ret.Jurisdictions = append(ret.Jurisdictions, j)
			}
		}
	}

	exclude := req.FormValue("exclude")
	ret.Aggregate = req.FormValue("breakout") != "true"
	switch {
	case len(ret.Jurisdictions) == 1 && !ret.Aggregate:
		ret.Country = ret.Jurisdictions[0].(*Jurisdiction)
		ret.Jurisdictions = make([]grumble.Persistable, 0)
		switch {
		case req.FormValue("include") != "":
			for ix, region := range strings.Split(req.FormValue("include"), ",") {
				if ix > 5 {
					break
				}
				r := ret.Country.GetRegion(region)
				if r != nil {
					ret.Jurisdictions = append(ret.Jurisdictions, r)
				}
			}
		default:
			if len(ret.Country.regions) > 1 {
				if ret.Jurisdictions, err = ret.Country.TopRegions(6, true, strings.Split(exclude, ",")); err != nil {
					return
				}
			}
		}
	case len(ret.Jurisdictions) == 1 && ret.Aggregate:
		ret.Country = ret.Jurisdictions[0].(*Jurisdiction)
		ret.Regions = make([]grumble.Persistable, 0)
		if len(ret.Country.regions) > 1 {
			switch {
			case req.FormValue("include") != "":
				for _, region := range strings.Split(req.FormValue("include"), ",") {
					r := ret.Country.GetRegion(region)
					if r != nil {
						ret.Regions = append(ret.Regions, r)
					}
				}
			case req.FormValue("exclude") != "":
				excl := make(map[int]bool, 0)
				for _, region := range strings.Split(req.FormValue("exclude"), ",") {
					r := ret.Country.GetRegion(region)
					if r != nil {
						excl[r.Ident] = true
					}
				}
				for _, r := range ret.Country.regions {
					if ok, _ := excl[r.Ident]; !ok {
						ret.Regions = append(ret.Regions, r)
					}
				}
			}
		}
	case len(ret.Jurisdictions) == 0:
		if ret.Jurisdictions, err = ret.TopCountries(6, true, strings.Split(exclude, ",")); err != nil {
			return
		}
	default:
		ret.Exclude = make([]grumble.Persistable, 0)
		for _, country := range strings.Split(exclude, ",") {
			if j := GetJurisdiction(country); j != nil {
				ret.Exclude = append(ret.Exclude, j)
			}
		}
	}
	ret.Query = ret.Manager.MakeQuery(Sample{})

	ret.ChartTypeCases = ChartTypeAbsolute
	if req.FormValue("cases") != "" {
		ret.ChartTypeCases = req.FormValue("cases")
	}
	ret.ChartTypeDeaths = ret.ChartTypeCases
	if req.FormValue("deaths") != "" {
		ret.ChartTypeDeaths = req.FormValue("deaths")
	}
	ret.Regression = false
	if (ret.ChartTypeCases == ChartTypeDaily) || (ret.ChartTypeCases == ChartTypeRollingAvg) {
		ret.ChartTypeDeaths = ret.ChartTypeCases
		if req.FormValue("regression") != "" {
			ret.Regression, _ = strconv.ParseBool(req.FormValue("regression"))
		}
	}
	return
}

func (data *CasesChartData) TopCountries(number int, cases bool, exclude []string) (countries []grumble.Persistable, err error) {
	excludes := make([]grumble.Persistable, 0)
	for _, excl := range exclude {
		if e := GetJurisdiction(excl); e != nil {
			excludes = append(excludes, e)
		}
	}
	q := data.Manager.MakeQuery(Sample{})
	q.AddCondition(&grumble.References{
		Column:     "Jurisdiction",
		References: excludes,
		Invert:     true,
	})
	q.AddCondition(&grumble.IsRoot{})
	q.AddCondition(&grumble.HasMaxValue{Column: "Date"})
	if cases {
		q.AddSort(grumble.Sort{Column: "Confirmed", Direction: "DESC"})
	} else {
		q.AddSort(grumble.Sort{Column: "Deceased", Direction: "DESC"})
	}
	q.AddReferenceJoins()
	q.Limit = number
	results, err := q.Execute()
	if err != nil {
		return
	}
	countries = make([]grumble.Persistable, 0)
	for _, row := range results {
		countries = append(countries, row[1])
	}
	return
}

func (data *CasesChartData) ExecuteQuery() (err error) {
	switch {
	case len(data.Jurisdictions) == 0:
		data.Query.AddCondition(&grumble.IsRoot{})
	case len(data.Regions) > 0 && data.Aggregate:
		data.Query.AddCondition(&grumble.References{
			Column:     "Jurisdiction",
			References: data.Regions,
		})
	default:
		data.Query.AddCondition(&grumble.References{
			Column:     "Jurisdiction",
			References: data.Jurisdictions,
		})
	}
	if len(data.Exclude) > 0 {
		data.Query.AddCondition(&grumble.References{
			Column:     "Jurisdiction",
			References: data.Exclude,
			Invert:     true,
		})
	}
	data.Query.AddSort(grumble.Sort{Column: "Date", Direction: "ASC"})
	data.Query.AddReferenceJoins()
	data.Results, err = data.Query.Execute()
	return
}

func (data *CasesChartData) BuildSeries() (err error) {
	data.Series = make(map[string]*DataSeries, 0)
	for _, row := range data.Results {
		sample := row[0].(*Sample)
		if data.First.Year() < 2019 {
			data.First = sample.Date
		}
		data.Last = sample.Date
		var jurisdiction *Jurisdiction
		var series *DataSeries
		switch {
		case len(data.Regions) > 0:
			jurisdiction = data.Jurisdictions[0].(*Jurisdiction)
		case len(data.Jurisdictions) > 0:
			jurisdiction = row[1].(*Jurisdiction)
		}
		series = data.GetDataSeries(jurisdiction, sample)
		series.AppendSample(sample)
	}
	for _, series := range data.Series {
		if series.Current != nil {
			series.Current.NewCount = series.Current.Count - series.Cases
			series.Current.NewDeceased = series.Current.Deceased - series.Deaths
		}
	}
	data.Last = data.Last.AddDate(0, 0, 1)
	data.Days = int(data.Last.Sub(data.First).Hours()) / 24
	return
}

func MakeCasesChartSeries(series *DataSeries, which int, chartType string, regression bool) (chartSeries *CasesChartSeries) {
	chartSeries = new(CasesChartSeries)
	chartSeries.Data = nil
	chartSeries.Current = 0
	chartSeries.New = 0
	chartSeries.Window = make([]float64, 7)
	chartSeries.WindowPtr = 0
	chartSeries.DataSeries = series
	chartSeries.Which = which
	chartSeries.ChartType = chartType
	chartSeries.Regression = regression
	return chartSeries
}

var subject = []string{"Confirmed", "Deceased"}

func (series *CasesChartSeries) Label(code string) string {
	switch series.ChartType {
	case ChartTypeRelative:
		return fmt.Sprintf("#%s/mio %s", subject[series.Which], code)
	case ChartTypeSuppress:
		return ""
	case ChartTypeDaily:
		return fmt.Sprintf("#Newly %s %s", subject[series.Which], code)
	case ChartTypeRollingAvg:
		return fmt.Sprintf("7 day rolling avg #newly %s %s", subject[series.Which], code)
	default:
		return fmt.Sprintf("#%s %s", subject[series.Which], code)
	}
}

func (series *CasesChartSeries) Append(ix int) {
	if series.Data == nil {
		series.Data = make([]float64, series.DataSeries.ChartData.Days)
	}
	switch series.ChartType {
	case ChartTypeRelative:
		series.Data[ix] = float64(series.Current) / (series.DataSeries.ChartData.Population / 1e6)
	case ChartTypeDaily:
		series.Data[ix] = float64(series.New)
	case ChartTypeRollingAvg:
		if series.WindowPtr < 6 {
			series.Data[ix] = 0.0
			series.Window[series.WindowPtr] = float64(series.New)
			series.WindowPtr += 1
		} else {
			sum := float64(series.New)
			for wix := 0; wix < 6; wix++ {
				sum += series.Window[wix]
				if wix > 0 {
					series.Window[wix-1] = series.Window[wix]
				}
			}
			series.Window[series.WindowPtr] = float64(series.New)
			series.Data[ix] = sum / 7.0
		}
	case ChartTypeSuppress:
		break
	case ChartTypeMortality:
		if series.Current > 0 {
			series.Data[ix] = float64(series.Current) / float64(series.DataSeries.ConfirmedData.Current)
		} else {
			series.Data[ix] = 0.0
		}
	default:
		series.Data[ix] = float64(series.Current)
	}
}

func (data *CasesChartData) BuildChart() (err error) {
	data.ChartSeries = make([]chart.Series, 0)
	sortedSeries := make([]*DataSeries, 0)
	for _, series := range data.Series {
		sortedSeries = append(sortedSeries, series)
	}
	sort.Slice(sortedSeries, func(i, j int) bool {
		return sortedSeries[i].Current.Count > sortedSeries[j].Current.Count
	})
	for _, series := range sortedSeries {
		dateSeries := make([]time.Time, data.Days)
		data.Population = 7.8e9
		if series.Jurisdiction != nil {
			data.Population = float64(series.Jurisdiction.Population)
		}
		code := ""
		if series.Jurisdiction != nil {
			code = series.Jurisdiction.Alpha3
			if code == "" {
				code = series.Jurisdiction.Alpha2
			}
		}
		caseLabel := series.ConfirmedData.Label(code)
		deathsLabel := series.DeceasedData.Label(code)
		seriesIx := 0
		for d, ix := data.First, 0; d.Before(data.Last); d, ix = d.AddDate(0, 0, 1), ix+1 {
			dateSeries[ix] = d
			if !d.Before(series.DataPoints[seriesIx].Date) {
				series.ConfirmedData.Current = series.DataPoints[seriesIx].Count
				series.ConfirmedData.New = series.DataPoints[seriesIx].NewCount
				series.DeceasedData.Current = series.DataPoints[seriesIx].Deceased
				series.DeceasedData.New = series.DataPoints[seriesIx].NewDeceased
				seriesIx++
			} else {
				series.ConfirmedData.New = 0
				series.DeceasedData.New = 0
			}
			series.ConfirmedData.Append(ix)
			series.DeceasedData.Append(ix)
		}
		if series.ConfirmedData.ChartType != ChartTypeSuppress {
			confirmedTimeSeries := chart.TimeSeries{
				Name: caseLabel,
				Style: chart.Style{
					StrokeColor: series.Color,
				},
				XValues: dateSeries,
				YValues: series.ConfirmedData.Data,
			}
			data.ChartSeries = append(data.ChartSeries, confirmedTimeSeries)

			if series.ConfirmedData.Regression {
				data.ChartSeries = append(data.ChartSeries, &chart.PolynomialRegressionSeries{
					Name: "Regression " + code,
					Style: chart.Style{
						StrokeColor:     series.Color,
						StrokeDashArray: []float64{2.0, 2.0},
					},
					Degree:      3,
					InnerSeries: confirmedTimeSeries,
				})
			}
		}
		if series.DeceasedData.ChartType != ChartTypeSuppress {
			yAxis := chart.YAxisPrimary
			var strokeDashArray []float64 = nil
			if series.ConfirmedData.ChartType != ChartTypeSuppress {
				yAxis = chart.YAxisSecondary
				strokeDashArray = []float64{5.0, 5.0}
			}
			deceasedTimeSeries := chart.TimeSeries{
				Name: deathsLabel,
				Style: chart.Style{
					StrokeColor:     series.Color,
					StrokeDashArray: strokeDashArray,
				},
				YAxis:   yAxis,
				XValues: dateSeries,
				YValues: series.DeceasedData.Data,
			}
			data.ChartSeries = append(data.ChartSeries, deceasedTimeSeries)
		}
	}
	return
}

func (data *CasesChartData) Render(res http.ResponseWriter) (err error) {
	graph := chart.Chart{
		Background: chart.Style{
			Padding: chart.Box{
				Top:    20,
				Left:   200,
				Bottom: 20,
			},
		},
		Series: data.ChartSeries,
	}

	graph.Elements = []chart.Renderable{
		chart.LegendLeft(&graph),
	}

	res.Header().Set("Content-Type", "image/png")
	err = graph.Render(chart.PNG, res)
	return
}

func CasesChart(res http.ResponseWriter, req *http.Request) {
	chartData, err := MakeCasesChartData(req)
	if err != nil {
		log.Printf("Error: %v", err)
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	if err = chartData.ExecuteQuery(); err != nil {
		log.Printf("Error: %v", err)
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	if err = chartData.BuildSeries(); err != nil {
		log.Printf("Error: %v", err)
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	if err = chartData.BuildChart(); err != nil {
		log.Printf("Error: %v", err)
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	if err = chartData.Render(res); err != nil {
		log.Printf("Error: %v", err)
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
}

func DeathsByPopulation(res http.ResponseWriter, req *http.Request) {
	mgr, err := grumble.MakeEntityManager()
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	_, newest, err := OldestAndNewestSample(mgr)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	q := mgr.MakeQuery(Sample{})
	q.AddCondition(&grumble.IsRoot{})
	q.AddFilter("Date", newest)
	q.AddSort(grumble.Sort{Column: "Confirmed", Direction: "DESC"})
	q.AddReferenceJoins()
	results, err := q.Execute()
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	populationSeries := make([]float64, 0)
	deathsSeries := make([]float64, 0)
	annotations := make([]chart.Value2, 0)
	for _, row := range results {
		sample := row[0].(*Sample)
		country := row[1].(*Jurisdiction)
		if country.Population < 1000 {
			continue
		}
		pop := float64(country.Population) / 1e6
		deathsByPop := float64(sample.Deceased) / pop
		if deathsByPop < 20 {
			continue
		}
		populationSeries = append(populationSeries, pop)
		deathsSeries = append(deathsSeries, deathsByPop)
		if deathsByPop > 50 {
			annotations = append(annotations,
				chart.Value2{XValue: pop, YValue: deathsByPop, Label: country.Alpha3})
		}
	}

	viridisByY := func(xr, yr chart.Range, index int, x, y float64) drawing.Color {
		return chart.Viridis(y, yr.GetMin(), yr.GetMax())
	}

	graph := chart.Chart{
		XAxis: chart.XAxis{
			Name: "Population (mio)",
		},
		YAxis: chart.YAxis{
			Name: "#Deceased/mio",
		},
		Series: []chart.Series{
			chart.ContinuousSeries{
				Style: chart.Style{
					StrokeWidth:      chart.Disabled,
					DotWidth:         5,
					DotColorProvider: viridisByY,
				},
				XValues: populationSeries,
				YValues: deathsSeries,
			},
			chart.AnnotationSeries{
				Annotations: annotations,
			},
		},
	}

	res.Header().Set("Content-Type", chart.ContentTypePNG)
	_ = graph.Render(chart.PNG, res)
}

func DeathsByGDP(res http.ResponseWriter, req *http.Request) {
	mgr, err := grumble.MakeEntityManager()
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	_, newest, err := OldestAndNewestSample(mgr)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	q := mgr.MakeQuery(Sample{})
	q.AddCondition(&grumble.IsRoot{})
	q.AddFilter("Date", newest)
	q.AddSort(grumble.Sort{Column: "Confirmed", Direction: "DESC"})
	q.AddReferenceJoins()
	results, err := q.Execute()
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	gdpSeries := make([]float64, 0)
	deathsSeries := make([]float64, 0)
	annotations := make([]chart.Value2, 0)
	for _, row := range results {
		sample := row[0].(*Sample)
		country := row[1].(*Jurisdiction)
		if country.GDPPerCapPPP < 10000 || country.Population < 10000 {
			continue
		}
		gdp := country.GDPPerCapPPP
		pop := float64(country.Population) / 1e6
		deathsByPop := float64(sample.Deceased) / pop
		if deathsByPop < 20 {
			continue
		}
		gdpSeries = append(gdpSeries, gdp)
		deathsSeries = append(deathsSeries, deathsByPop)
		if deathsByPop > 50 {
			annotations = append(annotations,
				chart.Value2{XValue: gdp, YValue: deathsByPop, Label: country.Alpha3})
		}
	}

	viridisByY := func(xr, yr chart.Range, index int, x, y float64) drawing.Color {
		return chart.Viridis(y, yr.GetMin(), yr.GetMax())
	}

	graph := chart.Chart{
		XAxis: chart.XAxis{
			Name: "GDP per capita PPP",
		},
		YAxis: chart.YAxis{
			Name: "#Deceased/mio",
		},
		Series: []chart.Series{
			chart.ContinuousSeries{
				Style: chart.Style{
					StrokeWidth:      chart.Disabled,
					DotWidth:         5,
					DotColorProvider: viridisByY,
				},
				XValues: gdpSeries,
				YValues: deathsSeries,
			},
			chart.AnnotationSeries{
				Annotations: annotations,
			},
		},
	}

	res.Header().Set("Content-Type", chart.ContentTypePNG)
	_ = graph.Render(chart.PNG, res)
}

func DeathsByMedianAge(res http.ResponseWriter, req *http.Request) {
	mgr, err := grumble.MakeEntityManager()
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	_, newest, err := OldestAndNewestSample(mgr)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}

	q := mgr.MakeQuery(Sample{})
	q.AddCondition(&grumble.IsRoot{})
	q.AddFilter("Date", newest)
	q.AddSort(grumble.Sort{Column: "Confirmed", Direction: "DESC"})
	q.AddReferenceJoins()
	results, err := q.Execute()
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	ageSeries := make([]float64, 0)
	deathsSeries := make([]float64, 0)
	annotations := make([]chart.Value2, 0)
	for _, row := range results {
		sample := row[0].(*Sample)
		country := row[1].(*Jurisdiction)
		if country.MedianAge < 20 || country.Population < 10000 {
			continue
		}
		age := country.MedianAge
		pop := float64(country.Population) / 1e6
		deathsByPop := float64(sample.Deceased) / pop
		if deathsByPop < 20 {
			continue
		}
		ageSeries = append(ageSeries, age)
		deathsSeries = append(deathsSeries, deathsByPop)
		if deathsByPop > 50 {
			annotations = append(annotations,
				chart.Value2{XValue: age, YValue: deathsByPop, Label: country.Alpha3})
		}
	}

	viridisByY := func(xr, yr chart.Range, index int, x, y float64) drawing.Color {
		return chart.Viridis(y, yr.GetMin(), yr.GetMax())
	}

	graph := chart.Chart{
		XAxis: chart.XAxis{
			Name: "Median Age",
		},
		YAxis: chart.YAxis{
			Name: "#Deceased/mio",
		},
		Series: []chart.Series{
			chart.ContinuousSeries{
				Style: chart.Style{
					StrokeWidth:      chart.Disabled,
					DotWidth:         5,
					DotColorProvider: viridisByY,
				},
				XValues: ageSeries,
				YValues: deathsSeries,
			},
			chart.AnnotationSeries{
				Annotations: annotations,
			},
		},
	}

	res.Header().Set("Content-Type", chart.ContentTypePNG)
	_ = graph.Render(chart.PNG, res)
}

type ChartPageContext struct {
	//
}

func (ipc *ChartPageContext) MakeContext(req *handler.PlainRequest) (err error) {
	data := make(map[string]interface{})
	req.Data = data
	req.Template = "html/charts.html"
	return
}

func ChartPage(w http.ResponseWriter, r *http.Request) {
	handler.ServePlainPage(w, r, &ChartPageContext{})
}
