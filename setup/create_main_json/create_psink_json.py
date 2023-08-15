import json
from dotenv import load_dotenv
import os
import math
import random
import ipaddress
import glob

# PSink
# ---------------
## Data Property
## * PSinkID
## * PSink Type (デバイスとクラスの対応づけ)
## * VPoint Module ID (VPoint moduleの実行系ファイル) <- 不要
## * Socket Address (VPoint のソケットファイル，本来はIP:Port) <- 不要
## * Serving IPv6 Prefix (PNode の接続用subnet)
## * Lat Lon
## * Description 
# ---------------
## Object Property
## * contains (Area->PSink)
## * isInstalledIn (PSink->Area)

load_dotenv()
json_file_path = os.getenv("PROJECT_PATH") + "/setup/GraphDB/config/"

# VPOINT_BASE_PORT
# PSINK_NUM_PER_AREA
# MIN_LAT, MAX_LAT, MIN_LON, MAX_LON
# AREA_WIDTH
# EDGE_SERVER_NUM
VPOINT_BASE_PORT = int(os.getenv("VPOINT_BASE_PORT"))
PSINK_NUM_PER_AREA = float(os.getenv("PSINK_NUM_PER_AREA"))
MIN_LAT = float(os.getenv("MIN_LAT"))
MAX_LAT = float(os.getenv("MAX_LAT"))
MIN_LON = float(os.getenv("MIN_LON"))
MAX_LON = float(os.getenv("MAX_LON"))
AREA_WIDTH = float(os.getenv("AREA_WIDTH"))
EDGE_SERVER_NUM = int(os.getenv("EDGE_SERVER_NUM"))

lineStep = AREA_WIDTH
forint = 1000

data = {"psinks":[]}

# 始点となるArea
swLat = MIN_LAT
neLat = swLat + lineStep

# label情報
label_lat = 0
label_lon = 0

# ID用のindex
id_index = 0

"""
# ソケットファイル群ファイルのリセット
socket_files_dir_path = os.getenv("PROJECT_PATH") + "/MECServer/VPoint/socket_files"
if not os.path.exists(socket_files_dir_path):
    os.makedirs(socket_files_dir_path)
else:
    files = glob.glob(f"{socket_files_dir_path}/*")
    for file in files:
        if os.path.isfile(file):
            os.remove(file)

# エッジサーバ分だけソケットファイルを作成
for i in range(EDGE_SERVER_NUM):
    full_path = socket_files_dir_path + "/vpoint_" + str(i+1) + ".json"
    socket_format_json = {
        "vpoints": []
    }
    with open(full_path, 'w') as f:
        json.dump(socket_format_json, f, indent=4)

# エッジサーバ数を長さとする，作成するソケットファイル数を数える配列
socket_file_num = [0] * EDGE_SERVER_NUM
VPOINT_NUM_PER_EDGE_SERVER_FOR_SOCK = 3
"""

# 左下からスタートし，右へ進んでいく
# 端まで到達したら一段上へ
while neLat <= MAX_LAT:
    swLon = MIN_LON
    neLon = swLon + lineStep
    label_lon = 0
    while neLon <= MAX_LON:
        label_area = "A" + str(label_lat) + ":" + str(label_lon)
        # PSINK_NUM_PER_AREA が 1以上か1より小さいかで分岐
        if PSINK_NUM_PER_AREA >= 1:
            i = 0
            while i < PSINK_NUM_PER_AREA:
                #data["psinks"].append({"psink":{}, "vpoint":{}})
                psinks = data["psinks"]

                # PSink情報の追加
                label_psink = "PS" + str(id_index)
                psink_id = str(int(0b0010 << 60) + id_index)
                #vpoint_id = str(int(0b1010 << 60) + id_index)
                #vpoint_module_id = os.getenv("PROJECT_PATH") + "/MECServer/VPoint/main"
                #socket_address = "/tmp/mecm2m/vpoint_" + str(server_num) + "_" + str(vpoint_id) + ".sock"
                random_ipv6 = ipaddress.IPv6Address(random.randint(0, 2**128 - 1))
                serving_ipv6_prefix = str(ipaddress.IPv6Network((random_ipv6, 64), strict=False))
                psink_lat = random.uniform(swLat, neLat)
                psink_lon = random.uniform(swLon, neLon)
                psink_dict = {
                    "property-label": "PSink",
                    "relation-label": {
                        "Area": label_area
                    },
                    "data-property": {
                        "Label": label_psink,
                        "PSinkID": psink_id,
                        "PSinkType": "",
                        #"VPointModuleID": vpoint_module_id,
                        #"SocketAddress": socket_address,
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
                psinks.append(psink_dict)

                """
                # VPointのソケットファイル群ファイルをここで作成
                if socket_file_num[server_num-1] < VPOINT_NUM_PER_EDGE_SERVER_FOR_SOCK:
                    full_path = socket_files_dir_path + "/vpoint_" + str(server_num) + ".json"
                    with open(full_path, 'r') as f:
                        socket_file_data = json.load(f)
                    socket_file_data["vpoints"].append(socket_address)
                    with open(full_path, 'w') as f:
                        json.dump(socket_file_data, f, indent=4)
                    
                    socket_file_num[server_num-1] += 1
                """
                
                id_index += 1
                i += 1
        # PSINK_NUM_PER_AREA が1より小さい場合，〜〜Areaに1個PSinkを設置
        else:
            interval = int(1 / PSINK_NUM_PER_AREA)
            if label_lon % interval == 0:
                psinks = data["psinks"]

                # PSink情報の追加
                label_psink = "PS" + str(id_index)
                psink_id = str(int(0b0010 << 60) + id_index)
                #vpoint_id = str(int(0b1010 << 60) + id_index)
                #vpoint_module_id = os.getenv("PROJECT_PATH") + "/MECServer/VPoint/main"
                #socket_address = "/tmp/mecm2m/vpoint_" + str(server_num) + "_" + str(vpoint_id) + ".sock"
                random_ipv6 = ipaddress.IPv6Address(random.randint(0, 2**128 - 1))
                serving_ipv6_prefix = str(ipaddress.IPv6Network((random_ipv6, 64), strict=False))
                psink_lat = random.uniform(swLat, neLat)
                psink_lon = random.uniform(swLon, neLon)
                psink_dict = {
                    "property-label": "PSink",
                    "relation-label": {
                        "Area": label_area
                    },
                    "data-property": {
                        "Label": label_psink,
                        "PSinkID": psink_id,
                        "PSinkType": "",
                        #"VPointModuleID": vpoint_module_id,
                        #"SocketAddress": socket_address,
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
                psinks.append(psink_dict)

                """
                # VPointのソケットファイル群ファイルをここで作成
                if socket_file_num[server_num-1] < VPOINT_NUM_PER_EDGE_SERVER_FOR_SOCK:
                    full_path = socket_files_dir_path + "/vpoint_" + str(server_num) + ".json"
                    with open(full_path, 'r') as f:
                        socket_file_data = json.load(f)
                    socket_file_data["vpoints"].append(socket_address)
                    with open(full_path, 'w') as f:
                        json.dump(socket_file_data, f, indent=4)
                    
                    socket_file_num[server_num-1] += 1
                """
                
                id_index += 1
        label_lon += 1
        swLon = ((swLon*forint) + (lineStep*forint)) / forint
        neLon = ((neLon*forint) + (lineStep*forint)) / forint
    label_lat += 1
    swLat = ((swLat*forint) + (lineStep*forint)) / forint
    neLat = ((neLat*forint) + (lineStep*forint)) / forint

psink_json = json_file_path + "config_main_psink.json"
with open(psink_json, 'w') as f:
    json.dump(data, f, indent=4)