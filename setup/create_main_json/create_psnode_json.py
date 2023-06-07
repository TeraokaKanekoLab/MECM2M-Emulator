import json
from dotenv import load_dotenv
import os
import math
import random
import string
import glob

# PSNode
# ---------------
## Data Property
## * PNodeID
## * PNode Type (デバイスとクラスの対応づけ)
## * VNode Module ID (VNode moduleの実行系ファイル)
## * Socket Address (VNode のソケットファイル，本来はIP:Port)
## * Lat Lon
## * Capability
## * Credential
## * Session Key
## * Description 
# ---------------
## Object Property
## * aggregates (PSink->PNode)
## * isConnectedTo (PNode->PSink)

# VSNode
# ---------------
## Data Property
## * VNodeID
## * Socket Address (VNode のソケットファイル，本来はIP:Port)
## * Software Module (VNode moduleの実行系ファイル)
## * Description
# ---------------
## Object Property
## * isVirtualizedBy (PNode->VNode)
## * isPhysicalizedBy (VNode->PNode)

# session key 生成
def generate_random_string(length):
    # 文字列に使用する文字を定義
    letters = string.ascii_letters + string.digits

    result_str = ''.join(random.choice(letters) for i in range(length))

    return result_str

load_dotenv()
json_file_path = os.getenv("PROJECT_PATH") + "/Main/config/json_files"

# VSNODE_BASE_PORT
# PSINK_NUM_PER_AREA
# PSNODE_NUM_PER_PSINK
# MIN_LAT, MAX_LAT, MIN_LON, MAX_LON
# AREA_WIDTH
# EDGE_SERVER_NUM
VSNODE_BASE_PORT = int(os.getenv("VSNODE_BASE_PORT"))
PSINK_NUM_PER_AREA = int(os.getenv("PSINK_NUM_PER_AREA"))
# ※ PSNodeは，PSinkに対して各種1つずつ
#PSNODE_NUM_PER_PSINK = 2
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

data = {"psnodes":[]}

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

# PSinkごとのPSNodeの番号
psnode_num = 0
port_num = 0

# ID用のindex
id_index = 0

# PNTypeをあらかじめ用意
pn_types = ["Temp_Sensor", "Humid_Sensor", "Anemometer"]
capabilities = {"Temp_Sensor":"MaxTemp", "Humid_Sensor":"MaxHumid", "Anemometer":"MaxWind"}
vsnode_psnode_relation = {}
for i in range(len(pn_types)):
    vsnode_psnode_relation[pn_types[i]] = []

# PSNodeのソケットファイル群ファイルのリセット
psnode_socket_files_dir_path = os.getenv("PROJECT_PATH") + "/PSNode/socket_files"
if not os.path.exists(psnode_socket_files_dir_path):
    os.makedirs(psnode_socket_files_dir_path)
else:
    files = glob.glob(f"{psnode_socket_files_dir_path}/*")
    for file in files:
        if os.path.isfile(file):
            os.remove(file)

# エッジサーバ分だけソケットファイルを作成
for i in range(EDGE_SERVER_NUM):
    full_path = psnode_socket_files_dir_path + "/psnode_" + str(i+1) + ".json"
    socket_format_json = {
        "psnodes": []
    }
    with open(full_path, 'w') as f:
        json.dump(socket_format_json, f, indent=4)

# VSNodeのソケットファイル群ファイルのリセット
vsnode_socket_files_dir_path = os.getenv("PROJECT_PATH") + "/MECServer/VSNode/socket_files"
if not os.path.exists(vsnode_socket_files_dir_path):
    os.makedirs(vsnode_socket_files_dir_path)
else:
    files = glob.glob(f"{vsnode_socket_files_dir_path}/*")
    for file in files:
        if os.path.isfile(file):
            os.remove(file)

# エッジサーバ分だけソケットファイルを作成
for i in range(EDGE_SERVER_NUM):
    full_path = vsnode_socket_files_dir_path + "/vsnode_" + str(i+1) + ".json"
    socket_format_json = {
        "vsnodes": []
    }
    with open(full_path, 'w') as f:
        json.dump(socket_format_json, f, indent=4)

# エッジサーバ数を長さとする，作成するソケットファイル数を数える配列
vsnode_socket_file_num = [0] * EDGE_SERVER_NUM
VSNODE_NUM_PER_EDGE_SERVER_FOR_SOCK = 3
PSNODE_NUM_PER_EDGE_SERVER_FOR_SOCK = 3
psnode_sock_num = 0

# 左下からスタートし，右へ進んでいく
# 端まで到達したら一段上へ
while neLat <= MAX_LAT:
    swLon = MIN_LON
    neLon = swLon + lineStep
    label_lon = 0
    while neLon <= MAX_LON:
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
            for pntype in range(len(pn_types)):
                vsnode_psnode_relation[pn_types[pntype]] = []
            label_psink = "PS" + str(server_num) + ":" + str(psink_num)
            psnode_num = 0
            j = 0
            while j < len(pn_types):
                data["psnodes"].append({"psnode":{}, "vsnode":{}})

                # PSNode情報の追加
                label_psnode = "PSN" + str(server_num) + ":" + str(psink_num) + ":" + str(psnode_num)
                psnode_id = str(int(0b0000 << 60) + id_index)
                vsnode_id = str(int(0b1000 << 60) + id_index)
                pnode_type = pn_types[j]
                vnode_module_id = os.getenv("PROJECT_PATH") + "/MECServer/VSNode/main"
                socket_address = "/tmp/mecm2m/vsnode_" + str(server_num) + "_" + str(vsnode_id) +".sock"
                psnode_lat = random.uniform(swLat, neLat)
                psnode_lon = random.uniform(swLon, neLon)
                capability = capabilities[pnode_type]
                credential = "YES"
                session_key = generate_random_string(10)
                psnode_description = "PSNode" + label_psnode
                psnode_dict = {
                    "property-label": "PSNode",
                    "relation-label": {
                        "Server": label_server,
                        "PSink": label_psink
                    },
                    "data-property": {
                        "Label": label_psnode,
                        "PNodeID": psnode_id,
                        "VNodeModuleID": vnode_module_id,
                        "SocketAddress": socket_address,
                        "Position": [round(psnode_lat, 4), round(psnode_lon, 4)],
                        "Capability": capability,
                        "Credential": credential,
                        "SessionKey": session_key,
                        "Description": psnode_description
                    },
                    "object-property": [
                        {
                            "from": {
                                "property-label": "PSNode",
                                "data-property": "Label",
                                "value": label_psnode
                            },
                            "to": {
                                "property-label": "PSink",
                                "data-property": "Label",
                                "value": label_psink
                            },
                            "type": "isConnectedTo"
                        },
                        {
                            "from": {
                                "property-label": "PSink",
                                "data-property": "Label",
                                "value": label_psink
                            },
                            "to": {
                                "property-label": "PSNode",
                                "data-property": "Label",
                                "value": label_psnode
                            },
                            "type": "aggregates"
                        }
                    ]
                }
                data["psnodes"][-1]["psnode"] = psnode_dict

                # PSNodeのソケットファイル群ファイルをここで作成
                if server_num == 1 and psnode_sock_num < PSNODE_NUM_PER_EDGE_SERVER_FOR_SOCK:
                    full_path = psnode_socket_files_dir_path + "/psnode_" + str(server_num) + ".json"
                    with open(full_path, 'r') as f:
                        socket_file_data = json.load(f)
                    new_socket_file_path = "/tmp/mecm2m/psnode_" + str(server_num) + "_" + str(psnode_id) + ".sock"
                    socket_file_data["psnodes"].append(new_socket_file_path)
                    with open(full_path, 'w') as f:
                        json.dump(socket_file_data, f, indent=4)
                    psnode_sock_num += 1

                # VSNode情報の追加
                label_vsnode = "VSN" + str(server_num) + ":" + str(psink_num) + ":" + str(psnode_num)
                port = VSNODE_BASE_PORT + port_num
                vsnode_description = "VSNode" + label_vsnode
                vsnode_dict = {
                    "property-label": "VSNode",
                    "data-property": {
                        "Label": label_vsnode,
                        "VNodeID": vsnode_id,
                        "SocketAddress": socket_address,
                        "SoftwareModule": vnode_module_id,
                        "Description": vsnode_description
                    },
                    "object-property": [
                        {
                            "from": {
                                "property-label": "PSNode",
                                "data-property": "Label",
                                "value": label_psnode
                            },
                            "to": {
                                "property-label": "VSNode",
                                "data-property": "Label",
                                "value": label_vsnode
                            },
                            "type": "isVirtualizedBy"
                        },
                        {
                            "from": {
                                "property-label": "VSNode",
                                "data-property": "Label",
                                "value": label_vsnode
                            },
                            "to": {
                                "property-label": "PSNode",
                                "data-property": "Label",
                                "value": label_psnode
                            },
                            "type": "isPhysicalizedBy"
                        }
                    ]
                }
                data["psnodes"][-1]["vsnode"] = vsnode_dict

                # VSNodeのソケットファイル群ファイルをここで作成
                if vsnode_socket_file_num[server_num-1] < VSNODE_NUM_PER_EDGE_SERVER_FOR_SOCK:
                    full_path = vsnode_socket_files_dir_path + "/vsnode_" + str(server_num) + ".json"
                    with open(full_path, 'r') as f:
                        socket_file_data = json.load(f)
                    socket_file_data["vsnodes"].append(socket_address)
                    with open(full_path, 'w') as f:
                        json.dump(socket_file_data, f, indent=4)
                    
                    vsnode_socket_file_num[server_num-1] += 1

                port_num += 1
                psnode_num += 1
                id_index += 1
                j += 1
            psink_num += 1
            i += 1
        label_lon += 1
        swLon = ((swLon*forint) + (lineStep*forint)) / forint
        neLon = ((neLon*forint) + (lineStep*forint)) / forint
    label_lat += 1
    swLat = ((swLat*forint) + (lineStep*forint)) / forint
    neLat = ((neLat*forint) + (lineStep*forint)) / forint


psnode_json = json_file_path + "/config_main_psnode.json"
with open(psnode_json, 'w') as f:
    json.dump(data, f, indent=4)