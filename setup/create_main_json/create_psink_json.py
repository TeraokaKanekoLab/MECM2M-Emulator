import json
from dotenv import load_dotenv
import os
import random
import ipaddress

# PSink
# ---------------
## Data Property
## * PSinkID
## * PSink Type (デバイスとクラスの対応づけ)
## * Socket Address (PSinkへのアクセス用)
## * Serving IPv6 Prefix (デバイスの接続用subnet)
## * MEC IPv6 Address (MEC ServerのIPv6アドレス)
## * Lat Lon
## * Description 
# ---------------
## Object Property
## * contains (PArea->PSink)
## * isInstalledIn (PSink->PArea)

load_dotenv()
json_file_path = os.getenv("PROJECT_PATH") + "/setup/GraphDB/config/"

# IP_ADDRESS
# PSINK_BASE_PORT
IP_ADDRESS = os.getenv("IP_ADDRESS")
PSINK_BASE_PORT = int(os.getenv("PSINK_BASE_PORT"))

data = {"psinks":[]}

# ID用のindex
psink_id_index = 0

config_main_area_path = os.getenv("PROJECT_PATH") + "/setup/GraphDB/config/config_main_area.json"
with open(config_main_area_path, 'r') as file:
    area_data = json.load(file)

area_array = area_data["areas"]["parea"]

for i, area_instance in enumerate(area_array):
    random_deploy = random.choice([True, False])
    if i == 0:
        random_deploy = True
    if random_deploy:
        # このPAreaにPSinkを配置する
        psinks = data["psinks"]
        parea_label = area_instance["data-property"]["Label"]
        psink_label = "PS" + str(psink_id_index)
        psink_id = str(int(0b0001 << 60) + psink_id_index)
        psink_type = "Router"
        psink_port = PSINK_BASE_PORT + psink_id_index
        socket_address = IP_ADDRESS + ":" + str(psink_port)
        random_ipv6 = ipaddress.IPv6Address(random.randint(0, 2**128 - 1))
        serving_ipv6_prefix = str(ipaddress.IPv6Network((random_ipv6, 64), strict=False))
        mec_ipv6_address = IP_ADDRESS
        area_swLat = area_instance["data-property"]["SW"][0]
        area_swLon = area_instance["data-property"]["SW"][1]
        area_neLat = area_instance["data-property"]["NE"][0]
        area_neLon = area_instance["data-property"]["NE"][1]
        psink_lat = random.uniform(area_swLat, area_neLat)
        psink_lon = random.uniform(area_swLon, area_neLon)
        psink_description = "Description:" + psink_label
        psink_dict = {
            "property-label": "PSink",
            "data-property": {
                "Label": psink_label,
                "PSinkID": psink_id,
                "PSinkType": psink_type,
                "SocketAddress": socket_address,
                "ServingIPv6Prefix": serving_ipv6_prefix,   # 適当なサブネットマスクを生成する
                "MECIPv6Address": mec_ipv6_address,
                "Position": [round(psink_lat, 4), round(psink_lon, 4)],
                "Description": psink_description
            },
            "object-property": [
                {
                    "from": {
                        "property-label": "PSink",
                        "data-property": "Label",
                        "value": psink_label
                    },
                    "to": {
                        "property-label": "PArea",
                        "data-property": "Label",
                        "value": parea_label
                    },
                    "type": "isInstalledIn"
                },
                {
                    "from": {
                        "property-label": "PArea",
                        "data-property": "Label",
                        "value": parea_label
                    },
                    "to": {
                        "property-label": "PSink",
                        "data-property": "Label",
                        "value": psink_label
                    },
                    "type": "contains"
                }
            ]
        }
        psinks.append(psink_dict)

        psink_id_index += 1

psink_json = json_file_path + "config_main_psink.json"
with open(psink_json, 'w') as f:
    json.dump(data, f, indent=4)