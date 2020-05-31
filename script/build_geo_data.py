
import codecs
import csv
import json

jurisdictions = {}

class Jurisdiction:
    def __init__(self, m):
        self.name = m["name"]
        self.alpha2 = m.get("alpha-2", "")
        self.alpha3 = m.get("alpha-3", "")
        self.population = 0
        self.medianage = 0.0
        self.GDPperCapPPP = 0.0
        self.regions = []
        self.sub = {}
        if "regions" in m:
            for r in m["regions"]:
                region = Jurisdiction(r)
                self.regions.append(region)
                self.sub[region.name] = region
                if region.alpha2 != "":
                    self.sub[region.alpha2] = region
                if region.alpha3 != "":
                    self.sub[region.alpha3] = region
                for a in region.alias:
                    self.sub[a] = region

        self.alias = []
        if "alias" in m:
            for a in m["alias"]:
                self.alias.append(a)


class JurisdictionEncoder(json.JSONEncoder):
    def default(self, obj):
        if isinstance(obj, Jurisdiction):
            ret = {
                "name": obj.name,
                "alpha-2": obj.alpha2,
                "alpha-3": obj.alpha3,
                "population": obj.population,
                "medianage": obj.medianage,
                "gdppercapppp": obj.GDPperCapPPP,
                "alias": obj.alias,
                "regions": obj.regions
            }
            return ret
        return json.JSONEncoder.default(self, obj)                


with codecs.open('../iso3166.json', 'r', 'utf-8') as iso3166:
    data = json.load(iso3166)
    countries = []
    for c in data:
        j = Jurisdiction(c)
        countries.append(j)
        jurisdictions[j.name] = j
        if j.alpha2 != "":
            jurisdictions[j.alpha2] = j
        if j.alpha3 != "":
            jurisdictions[j.alpha3] = j
        for a in j.alias:
            jurisdictions[a] = j


with open('../populations.json') as f:
    data = json.load(f)
    for c in data:
        if c["Country_Code"] in jurisdictions:
            j = jurisdictions[c["Country_Code"]]
            y = 2016
            while y >= 1960 and j.population == 0:
                p = c.get(f"Year_{y}")
                if p is not None:
                    j.population = round(p)
                y -= 1

us = jurisdictions["USA"]
with open('../us_state_populations.json') as f:
    data = json.load(f)
    for st in data:
        if st["name"] in us.sub:
            j = us.sub[st["name"]]
            j.population = st["population"]

with open('../medianage.json') as f:
    data = json.load(f)
    for c in data:
        j = None
        if "Code" in c and c["Code"] in jurisdictions:
            j = jurisdictions[c["Code"]]
        elif c["Name"] in jurisdictions:
            j = jurisdictions[c["Name"]]
        if j is not None:
            j.medianage = c["medianage"]

with open('../GDPperCapPPP.csv', newline='') as f:
    reader = csv.DictReader(f)
    for row in reader:
        code = row["Country Code"]
        if code in jurisdictions:
            j = jurisdictions[code]
            y = 2019
            while y >= 1960 and j.GDPperCapPPP == 0.0:
                p = row.get(f"{y}")
                if p is not None and p != "":
                    j.GDPperCapPPP = float(row[f"{y}"])
                y -= 1
        else:
            print("Miss", code)

with codecs.open("../countries.json", "w", "utf-8") as out:
    json.dump(countries, out, cls=JurisdictionEncoder, indent=2, ensure_ascii=False)
