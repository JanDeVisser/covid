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
	"database/sql"
	"errors"
	"fmt"
	"github.com/JanDeVisser/grumble"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Jurisdiction struct {
	grumble.Key
	Name         string
	Alpha2       string  `grumble:"verbose_name=ISO-3166-2 Code"`
	Alpha3       string  `grumble:"verbose_name=ISO-3166-3 Code"`
	Alias        string
	Aliases      []string
	regions      map[string]*Jurisdiction
	Population   int64
	MedianAge    float64 `grumble:"verbose_name=Median Age"`
	GDPPerCapPPP float64 `grumble:"verbose_name=GDP per capita w/ purchasing parity"`
}

var jurisdictionsByName = make(map[string]*Jurisdiction, 0)
var jurisdictions = make(map[int]*Jurisdiction, 0)

var regionsForId = make(map[int][]*Jurisdiction, 0)

func CacheJurisdictions(mgr *grumble.EntityManager) (err error) {
	ClearCaches()
	q := mgr.MakeQuery(Jurisdiction{})
	_, err = q.Execute()
	return
}

func ClearCaches() {
	jurisdictionsByName = make(map[string]*Jurisdiction, 0)
	jurisdictions = make(map[int]*Jurisdiction, 0)
	regionsForId = make(map[int][]*Jurisdiction, 0)
}

func persistJurisdictions(regions []Region) (err error) {
	mgr, err := grumble.MakeEntityManager()
	if err != nil {
		return
	}
	err = CacheJurisdictions(mgr)
	if err != nil {
		return
	}
	return mgr.TX(func(conn *sql.DB) (err error) {
		for _, c := range regions {
			if j, ok := jurisdictionsByName[c.Name]; ok {
				if j.Name == c.Name {
					if err = j.Sync(&c); err != nil {
						return
					}
				}
			} else {
				if j, err = c.Persist(mgr, nil); err != nil {
					return
				}
			}
		}
		return
	})
}

func (jurisdiction *Jurisdiction) Cache() {
	if _, ok := jurisdictions[jurisdiction.Id()]; !ok {
		jurisdictions[jurisdiction.Id()] = jurisdiction
	}

	var m map[string]*Jurisdiction = nil
	if jurisdiction.Parent() != nil && jurisdiction.Parent() != grumble.ZeroKey {
		pid := jurisdiction.Parent().Id()
		if p, ok := jurisdictions[pid]; ok {
			m = p.regions
		} else {
			regions, ok := regionsForId[pid]
			if !ok {
				regions = make([]*Jurisdiction, 0)
			}
			regionsForId[pid] = append(regions, jurisdiction)
		}
	} else {
		m = jurisdictionsByName
	}
	if m != nil {
		if _, ok := m[jurisdiction.Name]; !ok {
			m[jurisdiction.Name] = jurisdiction
			if jurisdiction.Alpha2 != ""{
				m[jurisdiction.Alpha2] = jurisdiction
			}
			if jurisdiction.Alpha3 != ""{
				m[jurisdiction.Alpha3] = jurisdiction
			}
			for _, alias := range jurisdiction.Aliases{
				m[alias] = jurisdiction
			}
		}
	}
	delete(regionsForId, jurisdiction.Id())
}

func (jurisdiction *Jurisdiction) Uncache() {
	var m map[string]*Jurisdiction = nil
	if jurisdiction.Parent() != nil && jurisdiction.Parent() != grumble.ZeroKey {
		if p, ok := jurisdictions[jurisdiction.Parent().Id()]; ok {
			m = p.regions
		}
	} else {
		m = jurisdictionsByName
	}
	if m != nil {
		delete(m, jurisdiction.Name)
		if jurisdiction.Alpha2 != "" {
			delete(m, jurisdiction.Alpha2)
		}
		if jurisdiction.Alpha3 != "" {
			delete(m, jurisdiction.Alpha3)
		}
		for _, alias := range jurisdiction.Aliases {
			delete(m, alias)
		}
	}
	delete(jurisdictions, jurisdiction.Id())
}

func (jurisdiction *Jurisdiction) Sync(region *Region) (err error) {
	log.Printf("Syncing %q", jurisdiction.Name)
	jurisdiction.Uncache()
	jurisdiction.Name = region.Name
	jurisdiction.Alpha2 = region.Alpha2
	jurisdiction.Alpha3 = region.Alpha3
	jurisdiction.Population = region.Population
	jurisdiction.MedianAge = region.MedianAge
	jurisdiction.GDPPerCapPPP = region.GDPPerCapPPP
	jurisdiction.Aliases = make([]string, len(region.Alias))
	copy(jurisdiction.Aliases, region.Alias)
	if err = jurisdiction.Manager().Put(jurisdiction); err != nil {
		return err
	}

	if err = jurisdiction.Manager().Put(jurisdiction); err != nil {
		return
	}
	regions := make(map[string]bool, 0)
	for _, sub := range region.Regions {
		if subj, ok := jurisdiction.regions[sub.Name]; !ok {
			subj, err = sub.Persist(jurisdiction.Manager(), jurisdiction)
		} else {
			err = subj.Sync(&sub)
		}
		if err != nil {
			return err
		}
		regions[sub.Name] = true
	}
	for name, subj := range jurisdiction.regions {
		if _, ok := regions[name]; !ok {
			delete(jurisdiction.regions, name)
			if err = jurisdiction.Manager().Delete(subj); err != nil {
				return
			}
		}
	}
	jurisdiction.Cache()
	return
}

func GetJurisdiction(name string) (ret *Jurisdiction) {
	if id, err := strconv.ParseInt(name, 10, 64); err == nil {
		return jurisdictions[int(id)]
	} else {
		return jurisdictionsByName[name]
	}
}

func (jurisdiction *Jurisdiction) GetRegion(name string) (ret *Jurisdiction) {
	ret = jurisdiction.regions[name]
	if ret == nil {
		ret = GetJurisdiction(name)
	}
	return
}

func (jurisdiction *Jurisdiction) GetFlag(size string) (flagURL string) {
	pdir := ""
	if jurisdiction.Parent() != grumble.ZeroKey && jurisdiction.Parent() != nil {
		p := jurisdictions[jurisdiction.Parent().Id()]
		pdir = strings.ToLower(fmt.Sprintf("%s/", p.Alpha2))
	}
	return fmt.Sprintf("/image/flags/%s/%s%s.png", size, pdir, strings.ToLower(jurisdiction.Alpha2))
}

func (jurisdiction *Jurisdiction) TopRegions(number int, cases bool, exclude []string) (regions []grumble.Persistable, err error) {
	excludes := make(map[int]bool, len(exclude))
	for _, excl := range exclude {
		if e, ok := jurisdiction.regions[excl]; ok {
			excludes[e.Id()] = true
		}
	}
	q := jurisdiction.Manager().MakeQuery(Sample{})
	allRegions := make([]grumble.Persistable, 0)
	for _, r := range jurisdiction.regions {
		if _, ok := excludes[r.Id()]; !ok {
			allRegions = append(allRegions, r)
		}
	}
	q.AddCondition(&grumble.References{
		Column:     "Jurisdiction",
		References: allRegions,
	})
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
	regions = make([]grumble.Persistable, 0)
	for _, row := range results {
		regions = append(regions, row[1])
	}
	return
}

func (jurisdiction *Jurisdiction) OnGet() (ret grumble.Persistable, err error) {
	ret = jurisdiction
	if jurisdiction.Alias != "" {
		jurisdiction.Aliases = strings.Split(jurisdiction.Alias, ";")
	}
	if cached, ok := jurisdictions[jurisdiction.Id()]; ok {
		jurisdiction.regions = cached.regions
	} else {
		jurisdiction.Cache()
	}
	return
}

func (jurisdiction *Jurisdiction) OnPut() (err error) {
	if jurisdiction.Aliases != nil {
		jurisdiction.Alias = strings.Join(jurisdiction.Aliases, ";")
	} else {
		jurisdiction.Alias = ""
	}
	return
}

func (jurisdiction *Jurisdiction) AfterPut() (err error) {
	return
}

func (jurisdiction *Jurisdiction) AfterCreate() (err error) {
	jurisdiction.regions = make(map[string]*Jurisdiction, 0)
	jurisdiction.Aliases = make([]string, 0)
	return
}

func (jurisdiction *Jurisdiction) MakeContext(data map[string]interface{}) (err error) {
	parameters := data["Parameters"].(url.Values)
	oldest, newest, err := OldestAndNewestSample(jurisdiction.Manager())
	if err != nil {
		log.Print(err)
		return
	}

	d := newest
	dstr := parameters.Get("date")
	if dstr != "" {
		if d, err = time.Parse("2006-01-02", dstr); err != nil {
			return
		}
	}
	data["date"] = d

	if len(jurisdiction.regions) > 0 {
		sampleQ := jurisdiction.Manager().MakeQuery(Sample{})
		sampleQ.AddFilter("Date", d)
		sampleQ.AddCondition(&grumble.References{
			Column:     "Jurisdiction",
			References: jurisdiction.AsKey(),
		})
		var sample grumble.Persistable
		sample, err = sampleQ.ExecuteSingle(nil)
		if err != nil {
			return
		}
		if sample == nil {
			return errors.New(fmt.Sprintf("No sample for jurisdiction %q and date %v", jurisdiction.Name, d))
		}

		q := jurisdiction.Manager().MakeQuery(Sample{})
		q.HasParent(sample)
		q.AddFilter("Date", d)
		q.AddSort(grumble.Sort{Column: "Confirmed", Direction: "DESC"})
		q.AddReferenceJoins()
		results, err := q.Execute()
		if err != nil {
			return err
		}
		data["regions"] = results

		dates := make([]time.Time, 0)
		for d := oldest; d.Before(newest); d = d.AddDate(0, 0, 1) {
			dates = append(dates, d)
		}
		data["daterange"] = append(dates, newest)
	}

	q := jurisdiction.Manager().MakeQuery(Sample{})
	q.AddCondition(&grumble.References{
		Column:     "Jurisdiction",
		References: jurisdiction.AsKey(),
	})
	q.AddSort(grumble.Sort{Column: "Date", Direction: "DESC"})
	results, err := q.Execute()
	cases := 0
	deaths := 0
	for ix := range results {
		s := results[len(results) - ix - 1][0].(*Sample)
		s.NewConfirmed = s.Confirmed - cases
		cases = s.Confirmed
		s.NewDeceased = s.Deceased - deaths
		deaths = s.Deceased
	}
	if err != nil {
		return err
	}
	data["dates"] = results

	return
}

func SyncCountriesRequest(res http.ResponseWriter, req *http.Request) {
	err := SyncCountries()
	if err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(res, req, "/index.html", http.StatusTemporaryRedirect)
}