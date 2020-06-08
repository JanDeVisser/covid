/*
 * This file is part of Covid.
 *
 * Copyright (c) 2020 Jan de Visser <jan@finiandarcy.com>
 *
 * Covid is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * Covid is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with Covid.  If not, see <https://www.gnu.org/licenses/>.
 */

package main

import (
	"database/sql"
	"github.com/JanDeVisser/covid/app"
	"github.com/JanDeVisser/grumble"
	"github.com/JanDeVisser/grumble/handler"
	"log"
	"net/http"
	"os"
)

func WebApp() {
	//handler.RegisterHandlerFnc("Index", IndexPage)
	handler.RegisterHandlerFnc("ChartCases", app.CasesChart)
	handler.RegisterHandlerFnc("ChartPage", app.ChartPage)
	handler.RegisterHandlerFnc("ChartDeathsByPop", app.DeathsByPopulation)
	handler.RegisterHandlerFnc("ChartDeathsByGDP", app.DeathsByGDP)
	handler.RegisterHandlerFnc("ChartDeathsByMedianAge", app.DeathsByMedianAge)
	handler.RegisterHandlerFnc("ImportSamples", app.ImportRequest)
	handler.RegisterHandlerFnc("Rebuild", app.RebuildRequest)
	handler.RegisterHandlerFnc("SyncCountries", app.SyncCountriesRequest)
	handler.RegisterHandlerFnc("ClearCache", ClearCacheRequest)
	handler.RegisterHandlerFnc("Wipe", WipeRequest)
	mgr, err := grumble.MakeEntityManager()
	if err != nil {
		log.Fatal(err)
	}
	if err := app.CacheJurisdictions(mgr); err != nil {
		log.Fatal(err)
	}
	handler.StartApp(true)
}

func ClearCacheRequest(res http.ResponseWriter, req *http.Request) {
	if err := os.RemoveAll("cache/"); err != nil {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(res, req, "/index.html", http.StatusTemporaryRedirect)
}

func WipeRequest(res http.ResponseWriter, req *http.Request) {
	if req.FormValue("magic") != "DEADBEEF" {
		http.Error(res, "Missing magic value", http.StatusInternalServerError)
	}
	if mgr, err := grumble.MakeEntityManager(); err == nil {
		if err = mgr.ResetSchema(); err == nil {
			if err = mgr.TX(func(db *sql.DB) error {
				for _, k := range grumble.Kinds() {
					if e := k.Reconcile(mgr.PostgreSQLAdapter); e != nil {
						return e
					}
				}
				return nil
			}); err != nil {
				http.Error(res, err.Error(), http.StatusInternalServerError)
			}
		} else {
			http.Error(res, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := app.SyncCountries(); err != nil {
		return
	}
	http.Redirect(res, req, "/import", http.StatusTemporaryRedirect)
}

func main() {
	grumble.GetKind(&app.Jurisdiction{})
	grumble.GetKind(&app.Sample{})
	grumble.GetKind(&app.ImportRecord{})
	WebApp()
}
