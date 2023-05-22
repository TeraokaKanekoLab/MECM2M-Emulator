import json
from dotenv import load_dotenv
import os
import math
import random
import ipaddress

# PSink
# ---------------
## Data Property
## * PSinkID
## * PSink Type (デバイスとクラスの対応づけ)
## * VPoint Module ID (VPoint moduleの実行系ファイル)
## * Socket Address (VPoint のソケットファイル，本来はIP:Port)
## * Serving IPv6 Prefix (PNode の接続用subnet)
## * Lat Lon
## * Description 
# ---------------
## Object Property
## * contains (Area->PSink)
## * isInstalledIn (PSink->Area)

# VPoint
# ---------------
## Data Property
## * VPointID
## * Socket Address (VPoint のソケットファイル，本来はIP:Port)
## * Software Module (VPoint moduleの実行系ファイル)
## * Description
# ---------------
## Object Property
## * isVirtualizedBy (PSink->VPoint)
## * isPhysicalizedBy (VPoint->PSink)

# 注意事項
# * PSink-VPointは一対一対応

load_dotenv()
json_file_path = os.getenv("PROJECT_PATH") + "/Main/config/json_files"

# VPOINT_BASE_PORT
# PSINK_NUM_PER_AREA
# MIN_LAT, MAX_LAT, MIN_LON, MAX_LON
# AREA_WIDTH
# EDGE_SERVER_NUM
VPOINT_BASE_PORT = int(os.getenv("VPOINT_BASE_PORT"))
PSINK_NUM_PER_AREA = int(os.getenv("PSINK_NUM_PER_AREA"))
MIN_LAT = float(os.getenv("MIN_LAT"))
MAX_LAT = float(os.getenv("MAX_LAT"))
MIN_LON = float(os.getenv("MIN_LON"))
MAX_LON = float(os.getenv("MAX_LON"))
AREA_WIDTH = float(os.getenv("AREA_WIDTH"))
EDGE_SERVER_NUM = int(os.getenv("EDGE_SERVER_NUM"))

lineStep = AREA_WIDTH
forint = 1000

area_num = math.ceil(((MAX_LAT-MIN_LAT)/AREA_WIDTH)*((MAX_LON-MIN_LON)/AREA_WIDTH))
area_num_per_server = int(area_num / EDGE_SERVER_NUM)

data = {"psinks":[]}

# 始点となるArea
swLat = MIN_LAT
neLat = swLat + lineStep

# label情報
label_lat = 0
label_lon = 0

# server_counter
server_counter = 0
server_num = 1

# ServerごとのPSinkの番号
psink_num = 0

# VPointのPort番号
port_num = 0

# ID用のindex
id_index = 0

# 左下からスタートし，右へ進んでいく
# 端まで到達したら一段上へ
while neLat <= MAX_LAT:
    swLon = MIN_LON
    neLon = swLon + lineStep
    label_lon = 0
    while neLon <= MAX_LON:
        label_area = "A" + str(label_lat) + ":" + str(label_lon)
        server_counter += 1
        if server_num > EDGE_SERVER_NUM:
            server_num -= 1
        if server_counter == area_num_per_server*server_num+1 and server_num < EDGE_SERVER_NUM:
            psink_num = 0
            server_num += 1
        if (area_num_per_server*(server_num-1)) <= server_counter < (area_num_per_server*server_num):
            label_server = "S" + str(server_num)
        i = 0
        while i < PSINK_NUM_PER_AREA:
            data["psinks"].append({"psink":{}, "vpoint":{}})

            # PSink情報の追加
            label_psink = "PS" + str(server_num) + ":" + str(psink_num)
            psink_id = str(int(0b0010 << 60) + id_index)
            vpoint_module_id = os.getenv("PROJECT_PATH") + "/MECServer/VPoint/main"
            socket_address = "/tmp/mecm2m/vpoint_" + str(server_num) + "_" + str(psink_num) + ".sock"
            random_ipv6 = ipaddress.IPv6Address(random.randint(0, 2**128 - 1))
            serving_ipv6_prefix = str(ipaddress.IPv6Network((random_ipv6, 64), strict=False))
            psink_lat = random.uniform(swLat, neLat)
            psink_lon = random.uniform(swLon, neLon)
            psink_dict = {
                "property-label": "PSink",
                "relation-label": {
                    "Server": label_server,
                    "Area": label_area
                },
                "data-property": {
                    "Label": label_psink,
                    "PSinkID": psink_id,
                    "PSinkType": "",
                    "VPointModuleID": vpoint_module_id,
                    "SocketAddress": socket_address,
                    "ServingIPv6Prefix": serving_ipv6_prefix,   # 適当なサブネットマスクを生成する
                    "Position": [round(psink_lat, 4), round(psink_lon, 4)],
                    "Description": "PSink" + label_psink
                },
                "object-property": [
                    {
                        "from": {
                            "property-label": "PSink",
                            "data-property": "Label",
                            "value": label_psink
                        },
                        "to": {
                            "property-label": "Area",
                            "data-property": "Label",
                            "value": label_area
                        },
                        "type": "isInstalledIn"
                    },
                    {
                        "from": {
                            "property-label": "Area",
                            "data-property": "Label",
                            "value": label_area
                        },
                        "to": {
                            "property-label": "PSink",
                            "data-property": "Label",
                            "value": label_psink
                        },
                        "type": "contains"
                    }
                ]
            }
            data["psinks"][-1]["psink"] = psink_dict

            # VPoint情報の追加
            label_vpoint = "VP" + str(server_num) + ":" + str(psink_num)
            vpoint_id = str(int(0b1010 << 60) + id_index)
            port = VPOINT_BASE_PORT + port_num
            vpoint_dict = {
                "property-label": "VPoint",
                "data-property": {
                    "Label": label_vpoint,
                    "VPointID": vpoint_id,
                    "SocketAddress": socket_address,
                    "SoftwareModule": vpoint_module_id,
                    "Description": "VPoint" + label_vpoint
                },
                "object-property": [
                    {
                        "from": {
                            "property-label": "VPoint",
                            "data-property": "Label",
                            "value": label_vpoint
                        },
                        "to": {
                            "property-label": "PSink",
                            "data-property": "Label",
                            "value": label_psink
                        },
                        "type": "isPhysicalizedBy"
                    },
                    {
                        "from": {
                            "property-label": "PSink",
                            "data-property": "Label",
                            "value": label_psink
                        },
                        "to": {
                            "property-label": "VPoint",
                            "data-property": "Label",
                            "value": label_vpoint
                        },
                        "type": "isVirtualizedBy"
                    }
                ]
            }
            data["psinks"][-1]["vpoint"] = vpoint_dict
            
            port_num += 1
            psink_num += 1
            id_index += 1
            i += 1
        label_lon += 1
        swLon = ((swLon*forint) + (lineStep*forint)) / forint
        neLon = ((neLon*forint) + (lineStep*forint)) / forint
    label_lat += 1
    swLat = ((swLat*forint) + (lineStep*forint)) / forint
    neLat = ((neLat*forint) + (lineStep*forint)) / forint

psink_json = json_file_path + "/config_main_psink.json"
with open(psink_json, 'w') as f:
    json.dump(data, f, indent=4)