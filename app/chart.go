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

type DataSeries struct {
	Jurisdiction *Jurisdiction
	Color        drawing.Color
	First        time.Time
	Last         time.Time
	Current      *DataPoint
	Cases        int
	Deaths       int
	DataPoints   []*DataPoint
}

const (
	ChartTypeAbsolute  = "ABS"
	ChartTypeRelative  = "REL"
	ChartTypeDaily     = "DAILY"
	ChartTypeSuppress  = "NONE"
	ChartTypeMortality = "MORTALITY"
)

const (
	ChartDataCases  = 0
	ChartDataDeaths = 1
)

type CasesChartSeries struct {
	ChartType  string
	Regression bool
	Data       []float64
	Current    int
	New        int
}

type CasesChartData struct {
	Manager       *grumble.EntityManager
	Query         *grumble.Query
	Country       *Jurisdiction
	Jurisdictions []grumble.Persistable
	Regions       []grumble.Persistable
	Exclude       []grumble.Persistable
	Aggregate     bool

	Results     [][]grumble.Persistable
	Series      map[string]*DataSeries
	First       time.Time
	Last        time.Time
	Days        int
	Population  float64
	Data        [2]CasesChartSeries
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

	ret.Data[ChartDataCases].ChartType = ChartTypeAbsolute
	if req.FormValue("cases") != "" {
		ret.Data[ChartDataCases].ChartType = req.FormValue("cases")
	}
	ret.Data[ChartDataDeaths].ChartType = ret.Data[ChartDataCases].ChartType
	if req.FormValue("deaths") != "" {
		ret.Data[ChartDataDeaths].ChartType = req.FormValue("deaths")
	}
	ret.Data[ChartDataCases].Regression = false
	if ret.Data[ChartDataCases].ChartType == ChartTypeDaily {
		ret.Data[ChartDataDeaths].ChartType = ChartTypeDaily
		if req.FormValue("regression") != "" {
			ret.Data[ChartDataCases].Regression, _ = strconv.ParseBool(req.FormValue("regression"))
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
	colix := 0
	for _, row := range data.Results {
		sample := row[0].(*Sample)
		if data.First.Year() < 2019 {
			data.First = sample.Date
		}
		data.Last = sample.Date
		var jurisdiction *Jurisdiction
		var series *DataSeries
		var ok bool
		var name string
		switch {
		case len(data.Regions) > 0:
			jurisdiction = data.Jurisdictions[0].(*Jurisdiction)
			name = jurisdiction.Name
		case len(data.Jurisdictions) > 0:
			jurisdiction = row[1].(*Jurisdiction)
			name = jurisdiction.Name
		default:
			name = "Global"
		}
		series, ok = data.Series[name]
		if !ok {
			series = new(DataSeries)
			series.Jurisdiction = jurisdiction
			series.Color = Colors[colix]
			colix++
			series.First = sample.Date
			series.Current = nil
			series.DataPoints = make([]*DataPoint, 0)
			data.Series[name] = series
		}
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

var subject = []string{"Confirmed", "Deceased"}

func (data *CasesChartData) label(which int, code string) string {
	switch data.Data[which].ChartType {
	case ChartTypeRelative:
		return fmt.Sprintf("#%s/mio %s", subject[which], code)
	case ChartTypeSuppress:
		return ""
	case ChartTypeDaily:
		return fmt.Sprintf("#Newly %s %s", subject[which], code)
	default:
		return fmt.Sprintf("#%s %s", subject[which], code)
	}
}

func (data *CasesChartData) append(ix int) {
	for which := ChartDataCases; which <= ChartDataDeaths; which++ {
		d := data.Data[which]
		switch d.ChartType {
		case ChartTypeRelative:
			d.Data[ix] = float64(d.Current) / (data.Population / 1e6)
		case ChartTypeDaily:
			d.Data[ix] = float64(d.New)
		case ChartTypeSuppress:
			break
		case ChartTypeMortality:
			if d.Current > 0 {
				d.Data[ix] = float64(d.Current) / float64(data.Data[ChartDataCases].Current)
			} else {
				d.Data[ix] = 0.0
			}
		default:
			d.Data[ix] = float64(d.Current)
		}
	}
}

func (data *CasesChartData) BuildChart() (err error) {
	data.ChartSeries = make([]chart.Series, 0)
	for _, series := range data.Series {
		dateSeries := make([]time.Time, data.Days)
		data.Data[ChartDataCases].Data = make([]float64, data.Days)
		data.Data[ChartDataDeaths].Data = make([]float64, data.Days)
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
		caseLabel := data.label(ChartDataCases, code)
		deathsLabel := data.label(ChartDataDeaths, code)
		data.Data[ChartDataCases].Current = 0
		data.Data[ChartDataCases].New = 0
		data.Data[ChartDataDeaths].Current = 0
		data.Data[ChartDataDeaths].New = 0
		seriesIx := 0
		for d, ix := data.First, 0; d.Before(data.Last); d, ix = d.AddDate(0, 0, 1), ix+1 {
			dateSeries[ix] = d
			if !d.Before(series.DataPoints[seriesIx].Date) {
				data.Data[ChartDataCases].Current = series.DataPoints[seriesIx].Count
				data.Data[ChartDataCases].New = series.DataPoints[seriesIx].NewCount
				data.Data[ChartDataDeaths].Current = series.DataPoints[seriesIx].Deceased
				data.Data[ChartDataDeaths].New = series.DataPoints[seriesIx].NewDeceased
				seriesIx++
			} else {
				data.Data[ChartDataCases].New = 0
				data.Data[ChartDataDeaths].New = 0
			}
			data.append(ix)
		}
		if data.Data[ChartDataCases].ChartType != ChartTypeSuppress {
			confirmedTimeSeries := chart.TimeSeries{
				Name: caseLabel,
				Style: chart.Style{
					StrokeColor: series.Color,
				},
				XValues: dateSeries,
				YValues: data.Data[ChartDataCases].Data,
			}
			data.ChartSeries = append(data.ChartSeries, confirmedTimeSeries)

			if data.Data[ChartDataCases].Regression {
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
		if data.Data[ChartDataDeaths].ChartType != ChartTypeSuppress {
			yAxis := chart.YAxisPrimary
			var strokeDashArray []float64 = nil
			if data.Data[ChartDataCases].ChartType != ChartTypeSuppress {
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
				YValues: data.Data[ChartDataDeaths].Data,
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
