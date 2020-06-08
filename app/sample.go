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
	"log"
	"net/url"
	"time"
)

type Sample struct {
	grumble.Key
	Jurisdiction *Jurisdiction
	Date         time.Time
	NewConfirmed int `grumble:"verbose_name=New Confirmed Cases;transient=true"`
	Confirmed    int `grumble:"verbose_name=Total Confirmed Cases"`
	NewDeceased  int `grumble:"verbose_name=Newly Deceased Cases;transient=true"`
	Deceased     int `grumble:"verbose_name=Total Deceased Cases"`
	Recovered    int
	subs         map[string]*Sample
	region       *Region
}

func OldestAndNewestSample(mgr *grumble.EntityManager) (oldest time.Time, newest time.Time, err error) {
	oldest = time.Now()
	newest = time.Now()
	q := mgr.MakeQuery(Sample{})
	q.AddCondition(&grumble.HasMaxValue{Column: "Date"})
	results, err := q.Execute()
	if err != nil {
		return
	}
	for _, row := range results {
		s := row[0].(*Sample)
		newest = s.Date
	}

	q = mgr.MakeQuery(Sample{})
	q.AddCondition(&grumble.HasMinValue{Column: "Date"})
	results, err = q.Execute()
	if err != nil {
		return
	}
	for _, row := range results {
		s := row[0].(*Sample)
		oldest = s.Date
	}

	log.Printf("Oldest samples: %v Newest samples %v", oldest, newest)
	return
}

func (sample *Sample) GetFlag(size string) (flagURL string) {
	r := sample
	for r.Parent() != nil {
		e, err := r.Parent().Self()
		if err != nil {
			return fmt.Sprintf("/image/flags/%s/%s.png", size, "aq")
		}
		r = e.(*Sample)
	}
	if r.Jurisdiction != nil {
		return sample.Jurisdiction.GetFlag(size)
	} else {
		return fmt.Sprintf("/image/flags/%s/%s.png", size, "aq")
	}
}

func (sample *Sample) ManyQuery(query *grumble.Query, values url.Values) (ret *grumble.Query) {
	ret = query
	_, newest, err := OldestAndNewestSample(query.Manager)
	if err != nil {
		log.Print(err)
		return
	}
	d := newest
	dstr := values.Get("date")
	if dstr != "" {
		if d, err = time.Parse("2006-01-02", dstr); err != nil {
			log.Print(err)
		}
	}
	ret.AddCondition(&grumble.IsRoot{})
	ret.AddFilter("Date", d)
	ret.AddSort(grumble.Sort{Column: "Confirmed", Direction: "DESC"})
	return
}

func (sample *Sample) AfterCreate() (err error) {
	sample.subs = make(map[string]*Sample, 0)
	return
}

func (sample *Sample) MakeListContext(req *handler.EntityRequest, data map[string]interface{}) (err error) {
	oldest, newest, err := OldestAndNewestSample(req.Manager)
	if err != nil {
		return
	}
	d := newest
	if dstr := req.Values.Get("date"); dstr != "" {
		if d, err = time.Parse("2006-01-02", dstr); err != nil {
			log.Print(err)
		}
	}

	data["date"] = d
	data["Country"] = req.Values.Get("country")
	data["Exclude"] = req.Values.Get("exclude")
	data["Cases"] = req.Values.Get("cases")
	data["Deaths"] = req.Values.Get("deaths")
	data["Regression"] = req.Values.Get("regression")
	dates := make([]time.Time, 0)
	for d := oldest; d.Before(newest); d = d.AddDate(0, 0, 1) {
		dates = append(dates, d)
	}
	data["dates"] = append(dates, newest)
	return
}
