from dotenv import load_dotenv
import os
import argparse
import pandas as pd

load_dotenv()
min_lat = float(os.getenv("MIN_LAT"))
max_lat = float(os.getenv("MAX_LAT"))
min_lon = float(os.getenv("MIN_LON"))
max_lon = float(os.getenv("MAX_LON"))

parser = argparse.ArgumentParser()

# エリア数を指定するコマンドライン引数
parser.add_argument("num", type=int)
args = parser.parse_args()

nelat_array = []
nelon_array = []
swlat_array = []
swlon_array = []

if args.num == 1:
    file_name = "area_1_1_own.csv"
    current_lat = min_lat
    current_lon = min_lon
    step = 0.001
    while current_lat <= max_lat - step:
        nelat_array.append(round(current_lat+step, 3))
        nelon_array.append(round(current_lon+step, 3))
        swlat_array.append(round(current_lat, 3))
        swlon_array.append(round(current_lon, 3))
        current_lat += step
        current_lon += step
elif args.num == 5:
    file_name = "area_5_5_own.csv"
    current_lat = min_lat
    current_lon = min_lon
    step = 0.005
    while current_lat <= max_lat:
        nelat_array.append(round(current_lat+step, 3))
        nelon_array.append(round(current_lon+step, 3))
        swlat_array.append(round(current_lat, 3))
        swlon_array.append(round(current_lon, 3))
        current_lat += step
        current_lon += step
elif args.num == 10:
    file_name = "area_10_10_own.csv"
    current_lat = min_lat
    current_lon = min_lon
    step = 0.010
    while current_lat <= max_lat - step:
        nelat_array.append(round(current_lat+step, 3))
        nelon_array.append(round(current_lon+step, 3))
        swlat_array.append(round(current_lat, 3))
        swlon_array.append(round(current_lon, 3))
        current_lat += step
        current_lon += step

data = {
    "ne_lat": nelat_array,
    "ne_lon": nelon_array,
    "sw_lat": swlat_array,
    "sw_lon": swlon_array
}

df = pd.DataFrame(data)
df.to_csv(file_name, index=False, header=False)