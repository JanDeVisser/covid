import json

with open('iso3166.json') as iso3166:
    for c in



with open('populations.json') as f:
    with open('update_populations.sql', 'w+') as out:
        data = json.load(f)
        for c in data:
            print(f"UPDATE covid.jurisdiction SET \"Population\" = {c['Year_2016']} WHERE \"Alpha3\" = '{c['Country_Code']}';", file=out)
