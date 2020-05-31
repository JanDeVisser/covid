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
	"encoding/json"
	"github.com/JanDeVisser/grumble"
	"io/ioutil"
	"log"
)

type Region struct {
	Name         string
	Alpha2       string		`json:"alpha-2"`
	Alpha3       string     `json:"alpha-3"`
	Alias        []string
	Regions      []Region
	Population   int64
	MedianAge	 float64
	GDPPerCapPPP float64
}

func (region *Region) Persist(mgr *grumble.EntityManager, parent *Jurisdiction) (j *Jurisdiction, err error) {
	log.Printf("Creating %q", region.Name)
	pkey := grumble.ZeroKey
	if parent != nil {
		pkey = parent.AsKey()
	}
	je, err := mgr.New(Jurisdiction{}, pkey)
	if err != nil {
		return
	}
	j = je.(*Jurisdiction)
	j.Name = region.Name
	j.Alpha2 = region.Alpha2
	j.Alpha3 = region.Alpha3
	j.Population = region.Population
	j.MedianAge = region.MedianAge
	j.GDPPerCapPPP = region.GDPPerCapPPP
	if region.Alias != nil && len(region.Alias) > 0 {
		j.Aliases = make([]string, len(region.Alias))
		copy(j.Aliases, region.Alias)
	}
	if err = mgr.Put(j); err != nil {
		return
	}
	j.Cache()
	for _, sub := range region.Regions {
		_, err = sub.Persist(mgr, j)
		if err != nil {
			return
		}
	}
	return
}

/* ================================================================================================================ */

func ReadCountries() (countries []Region, err error) {
	log.Println("Reading country data")
	jsonText, err := ioutil.ReadFile("countries.json")
	if err != nil {
		return
	}
	if err = json.Unmarshal(jsonText, &countries); err != nil {
		return
	}
	return
}

func SyncCountries() (err error){
	countries, err := ReadCountries()
	if err != nil {
		return
	}
	return persistJurisdictions(countries)
}

