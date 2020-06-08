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
	"bytes"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"github.com/JanDeVisser/grumble"
	"github.com/JanDeVisser/grumble/handler"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type ImportRecord struct {
	grumble.Key
	Timestamp  time.Time
	JHUFile    string
	Count      int
	ErrorCount int
	Errors     string
}

func download(report string) (data []byte, err error) {
	log.Printf("Downloading %s", report)
	resp, err := http.Get(report)
	if err != nil {
		return
	}
	defer func() {
		e := resp.Body.Close()
		if err == nil {
			err = e
		}
	}()
	if resp.StatusCode == http.StatusOK {
		data, err = ioutil.ReadAll(resp.Body)
	} else {
		data, err = ioutil.ReadAll(resp.Body)
		err = errors.New(string(data))
	}
	return
}

func parseInt(s string) int {
	if s == "" {
		return 0
	}
	if len(s) > 2 && s[len(s)-2:] == ".0" {
		s = s[:len(s)-2]
	}
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		log.Fatal(err)
	}
	return int(i)
}

func parseFloat(s string) float64 {
	if s == "" {
		return 0.0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Fatal(err)
	}
	return f
}

var samples map[string]*Sample

func getSample(mgr *grumble.EntityManager, parent *Sample, d time.Time, j *Jurisdiction, r []string) (s *Sample, err error) {
	var ok bool
	pk := grumble.ZeroKey
	if parent == nil {
		s, ok = samples[j.Name]
	} else {
		pk = parent.AsKey()
		s, ok = parent.subs[j.Name]
	}
	if !ok {
		var e grumble.Persistable
		e, err = mgr.New(Sample{}, pk)
		if err != nil {
			return
		}
		s = e.(*Sample)
		s.Jurisdiction = j
		s.Date = d
		if parent != nil {
			parent.subs[j.Name] = s
		} else {
			samples[j.Name] = s
		}
	}
	switch len(r) {
	case 6:
		s.Confirmed += parseInt(r[3])
		s.Deceased += parseInt(r[4])
		s.Recovered += parseInt(r[5])
	case 8:
		s.Confirmed += parseInt(r[3])
		s.Deceased += parseInt(r[4])
		s.Recovered += parseInt(r[5])
	default:
		s.Confirmed += parseInt(r[7])
		s.Deceased += parseInt(r[8])
		s.Recovered += parseInt(r[9])
	}
	return
}

var unknownRegionsByCountry = make(map[string][]string)

func unknownRegion(j *Jurisdiction, region string) {
	regions, ok := unknownRegionsByCountry[j.Alpha2]
	if !ok {
		regions = make([]string, 0)
	}
	regions = append(regions, region)
	unknownRegionsByCountry[j.Alpha2] = regions
}

func importRecord(mgr *grumble.EntityManager, d time.Time, r []string) (err error) {
	var admin2 string
	var provState string
	var countryName string
	switch len(r) {
	case 6, 8:
		provState = r[0]
		countryName = r[1]
		switch {
		case provState == countryName:
			provState = ""
		case provState == "Chicago":
			provState = "IL"
		case provState == "Washington, D.C.":
			provState = "DC"
		case provState == "Taiwan":
			provState = ""
			countryName = "Taiwan"
		case strings.HasSuffix(countryName, " SAR"):
			provState = ""
		}
	default:
		admin2 = r[1]
		provState = r[2]
		countryName = r[3]
	}
	countryName = strings.TrimSpace(countryName)
	provState = strings.TrimSpace(provState)

	if provState == "None" || provState == "Unknown" {
		provState = ""
	}
	if countryName == "Others" ||
		provState == "Wuhan Evacuee" ||
		strings.Contains(provState, "Recovered") ||
		strings.Contains(provState, "Diamond Princess") ||
		strings.Contains(countryName, "Diamond Princess") ||
		strings.Contains(provState, "Grand Princess") ||
		strings.Contains(countryName, "Grand Princess") ||
		strings.Contains(provState, "MS Zaandam") ||
		strings.Contains(countryName, "MS Zaandam") ||
		strings.Contains(provState, "Cruise") ||
		strings.Contains(countryName, "Cruise") {
		// FIXME
		return
	}

	if strings.HasSuffix(provState, ", Alberta") {
		provState = "AB"
	}

	if len(provState) > 4 && provState[len(provState)-4:len(provState)-2] == ", " {
		provState = provState[len(provState)-2:]
	}

	var c *Sample
	if countryName == "" {
		log.Printf("No country name in record %v", r)
		return errors.New(fmt.Sprintf("No country name for %s %s", admin2, provState))
	}
	country := GetJurisdiction(countryName)
	if country == nil {
		log.Printf("country for %q not found", countryName)
		return errors.New(fmt.Sprintf("country for %q not found", countryName))
	}
	c, err = getSample(mgr, nil, d, country, r)
	if err != nil {
		return
	}

	if provState != "" {
		region := country.GetRegion(provState)
		if region != nil {
			_, err = getSample(mgr, c, d, region, r)
			if err != nil {
				return
			}
		} else {
			unknownRegion(country, provState)
			//log.Printf("Region %q in country %q not found", provState, countryName)
			//return errors.New(fmt.Sprintf("region %q in country %q not found", provState, countryName))
		}
	}
	return
}

func putSample(s *Sample) (err error) {
	//log.Printf("Storing %s", s.Jurisdiction)
	if err = s.Manager().Put(s); err != nil {
		return
	}
	for _, sub := range s.subs {
		if err = putSample(sub); err != nil {
			return
		}
	}
	return
}

func importSamples(mgr *grumble.EntityManager, from *time.Time, to *time.Time, save bool) (err error) {
	var d time.Time
	var end time.Time

	if err = mgr.TX(func(db *sql.DB) (err error) {
		_ = os.Mkdir("cache", 0777)
		d = time.Date(2020, 1, 22, 0, 0, 0, 0, time.Local)
		if from != nil {
			d = *from
		} else {
			q := mgr.MakeQuery(ImportRecord{})
			results, err := q.Execute()
			if err != nil {
				return err
			}
			for _, row := range results {
				if importDate, err := time.Parse("01-02-2006", row[0].(*ImportRecord).JHUFile); err != nil {
					return err
				} else {
					if importDate.After(d) {
						d = importDate
					}
				}
			}
			d.AddDate(0, 0, 1)
		}
		end = time.Now()
		if to != nil {
			end = *to
		}
		return
	}); err != nil {
		return
	}
	for ; end.After(d); d, err = d.AddDate(0, 0, 1), nil {
		samples = make(map[string]*Sample, 0)

		var data []byte = nil

		fname := d.Format("01-02-2006")

		var ie grumble.Persistable
		if err = mgr.TX(func(db *sql.DB) (err error) {
			if ie, err = mgr.By(ImportRecord{}, "JHUFile", fname); err != nil {
				log.Printf("Error finding import record for %q: %v", fname, err)
				return
			}
			return
		}); err != nil {
			return
		}
		if ie != nil {
			log.Printf("Data for %v already imported", d)
			continue
		}

		useCache := false
		if cacheIface, ok := handler.GetAppConfig()["usecache"]; ok {
			useCache = cacheIface.(bool)
		}
		cachefile := fmt.Sprintf("cache/%s.csv", fname)
		if useCache {
			var f *os.File
			f, err = os.Open(cachefile)
			if err == nil {
				log.Printf("Reading cache file %q", cachefile)
				data, err = ioutil.ReadAll(f)
				if err != nil {
					log.Printf("Error reading cached report %q: %v", fname, err)
					_ = os.Remove(cachefile)
					data = nil
				}
				_ = f.Close()
			}
		}
		if data == nil {
			report := fmt.Sprintf(
				"https://raw.github.com/CSSEGISandData/COVID-19/master/csse_covid_19_data/csse_covid_19_daily_reports/%s.csv",
				fname)
			data, err = download(report)
			if err != nil {
				log.Printf("Error downloading %q: %v", fname, err)
				continue
			}
			if useCache {
				f, err := os.Create(cachefile)
				if err == nil {
					_, _ = f.Write(data)
					_ = f.Close()
				}
			}
		}

		r := csv.NewReader(bytes.NewReader(data))
		var records [][]string
		records, err = r.ReadAll()
		if err != nil {
			log.Printf("Error reading CSV data for %q: %v", fname, err)
			if useCache {
				_ = os.Remove(cachefile)
			}
			continue
		}

		errorCount := 0
		good := 0
		errorMessages := make([]string, 0)
		err = mgr.TX(func(db *sql.DB) error {
			for ix, r := range records[1:] {
				if err = importRecord(mgr, d, r); err != nil {
					s := fmt.Sprintf("%s:%d: %v", fname, ix+1, err)
					errorMessages = append(errorMessages, s)
					log.Print(s)
					errorCount++
				} else {
					good++
				}
			}
			return nil
		})

		if errorCount == 0 {
			if save {
				err = mgr.TX(func(db *sql.DB) error {
					for _, s := range samples {
						if err = putSample(s); err != nil {
							return err
						}
					}
					return nil
				})
				if err != nil {
					s := fmt.Sprintf("Error writing samples for %q: %v", fname, err)
					errorMessages = append(errorMessages, s)
					log.Printf(s)
				}
			}

			if ie, err = mgr.New(ImportRecord{}, grumble.ZeroKey); err != nil {
				log.Printf("Error creating import record for %q: %v", fname, err)
				return
			} else {
				imp := ie.(*ImportRecord)
				imp.Timestamp = time.Now()
				imp.JHUFile = fname
				imp.Count = good
				imp.ErrorCount = errorCount
				imp.Errors = strings.Join(errorMessages, "\n")
				if err = mgr.Put(imp); err != nil {
					log.Printf("Error writing import record for %q: %v", fname, err)
					return
				}
			}
		}
	}
	for country, regions := range unknownRegionsByCountry {
		log.Printf("Unknown in %s", country)
		for _, r := range regions {
			log.Printf("\t%s", r)
		}
	}
	return
}

func ImportRequest(res http.ResponseWriter, req *http.Request) {
	mgr, err := grumble.MakeEntityManager()
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	err = importSamples(mgr, nil, nil, true)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(res, req, "/index.html", http.StatusTemporaryRedirect)
}

func RebuildRequest(res http.ResponseWriter, req *http.Request) {
	if req.FormValue("magic") != "DEADBEEF" {
		http.Error(res, "Missing magic value", http.StatusInternalServerError)
	}
	mgr, err := grumble.MakeEntityManager()
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	ClearCaches()
	if err = grumble.GetKind(Jurisdiction{}).Truncate(mgr.PostgreSQLAdapter); err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	if err = grumble.GetKind(Sample{}).Truncate(mgr.PostgreSQLAdapter); err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	if err = grumble.GetKind(ImportRecord{}).Truncate(mgr.PostgreSQLAdapter); err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	err = SyncCountries()
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	err = importSamples(mgr, nil, nil, true)
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(res, req, "/index.html", http.StatusTemporaryRedirect)
}
