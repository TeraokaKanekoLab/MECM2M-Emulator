import json
from dotenv import load_dotenv
import os
import random
import string

# PSNode
# ---------------
## Data Property
## * PNodeID
## * PNode Type (デバイスとクラスの対応づけ)
## * VNode Module (VNode moduleの実行系ファイル)
## * Socket Address (デバイスへのアクセス用，本来はIP:Port)
## * Lat Lon
## * Capability
## * Credential
## * Session Key
## * Description 
# ---------------
## Object Property
## * aggregates (PSink->PNode)
## * isConnectedTo (PNode->PSink)
## * contains (PArea->PNode)
## * isInstalledIn (PNode->PArea)

# VSNode
# ---------------
## Data Property
## * VNodeID
## * Socket Address (モジュールへのアクセス用，本来はIP:Port)
## * Software Module (VNode moduleの実行系ファイル)
## * Description
# ---------------
## Object Property
## * isVirtualizedBy (PNode->VNode)
## * isPhysicalizedBy (VNode->PNode)
## * contains (PArea->VNode)
## * isInstalledIn (VNode->PArea)

# session key 生成
def generate_random_string(length):
    # 文字列に使用する文字を定義
    letters = string.ascii_letters + string.digits

    result_str = ''.join(random.choice(letters) for i in range(length))

    return result_str

def generate_psnode_dict():
    # PSNode情報の追加
    parea_label = "PA" + str(label_lat) + ":" + str(label_lon)
    psnode_label = "PSN" + str(id_index)
    psnode_id = str(int(0b0010 << 60) + id_index)
    pnode_type = random.choice(pn_types)
    vnode_module = os.getenv("PROJECT_PATH") + "/VSNode/main"
    psnode_port = PSNODE_BASE_PORT + id_index
    pnode_socket_address = IP_ADDRESS + ":" + str(psnode_port)
    psnode_lat = random.uniform(swLat, neLat)
    psnode_lon = random.uniform(swLon, neLon)
    capability = capabilities[pnode_type]
    credential = "YES"
    session_key = generate_random_string(10)
    psnode_description = "Description:" + psnode_label
    psnode_dict = {
        "property-label": "PSNode",
        "data-property": {
            "Label": psnode_label,
            "PNodeID": psnode_id,
            "PNodeType": pnode_type,
            "VNodeModule": vnode_module,
            "SocketAddress": pnode_socket_address,
            "Position": [round(psnode_lat, 4), round(psnode_lon, 4)],
            "Capability": capability,
            "Credential": credential,
            "SessionKey": session_key,
            "Description": psnode_description
        },
        "object-property": [
            {
                "from": {
                    "property-label": "PArea",
                    "data-property": "Label",
                    "value": parea_label
                },
                "to": {
                    "property-label": "PSNode",
                    "data-property": "Label",
                    "value": psnode_label
                },
                "type": "contains"
            },
            {
                "from": {
                    "property-label": "PSNode",
                    "data-property": "Label",
                    "value": psnode_label
                },
                "to": {
                    "property-label": "PArea",
                    "data-property": "Label",
                    "value": parea_label
                },
                "type": "isInstalledIn"
            }
        ]
    }

    # PSNode/initial_environment.json に初期環境に配置されるPSNodeのポート番号を格納
    initial_environment_file = psnode_dir_path + "initial_environment.json"
    with open(initial_environment_file, 'r') as file:
        ports_data = json.load(file)
    ports_data["ports"].append(psnode_port)
    with open(initial_environment_file, 'w') as file:
        json.dump(ports_data, file, indent=4)

    return psnode_dict

def generate_vsnode_dict():
    # VSNode情報の追加
    parea_label = "PA" + str(label_lat) + ":" + str(label_lon)
    psnode_label = "PSN" + str(id_index)
    vsnode_id = str(int(0b1000 << 60) + id_index)
    vsnode_port = VSNODE_BASE_PORT + id_index
    vnode_socket_address = IP_ADDRESS + ":" + str(vsnode_port)
    vnode_module = os.getenv("PROJECT_PATH") + "/VSNode/main"
    vsnode_label = "VSN" + str(id_index)
    vsnode_description = "Description:" + vsnode_label
    vsnode_dict = {
        "property-label": "VSNode",
        "data-property": {
            "Label": vsnode_label,
            "VNodeID": vsnode_id,
            "SocketAddress": vnode_socket_address,
            "SoftwareModule": vnode_module,
            "Description": vsnode_description
        },
        "object-property": [
            {
                "from": {
                    "property-label": "PSNode",
                    "data-property": "Label",
                    "value": psnode_label
                },
                "to": {
                    "property-label": "VSNode",
                    "data-property": "Label",
                    "value": vsnode_label
                },
                "type": "isVirtualizedBy"
            },
            {
                "from": {
                    "property-label": "VSNode",
                    "data-property": "Label",
                    "value": vsnode_label
                },
                "to": {
                    "property-label": "PSNode",
                    "data-property": "Label",
                    "value": psnode_label
                },
                "type": "isPhysicalizedBy"
            },
            {
                "from": {
                    "property-label": "VSNode",
                    "data-property": "Label",
                    "value": vsnode_label
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
                    "property-label": "VSNode",
                    "data-property": "Label",
                    "value": vsnode_label
                },
                "type": "contains"
            }
        ]
    }

    # VSNode/initial_environment.json に初期環境に配置されるVSNodeのポート番号を格納
    initial_environment_file = vsnode_dir_path + "initial_environment.json"
    with open(initial_environment_file, 'r') as file:
        ports_data = json.load(file)
    ports_data["ports"].append(vsnode_port)
    with open(initial_environment_file, 'w') as file:
        json.dump(ports_data, file, indent=4)

    return vsnode_dict

load_dotenv()
json_file_path = os.getenv("PROJECT_PATH") + "/setup/GraphDB/config/"

# VSNODE_BASE_PORT, PSNODE_BASE_PORT
# PSNODE_NUM_PER_AREA, AREA_PER_PSNODE_NUM
# MIN_LAT, MAX_LAT, MIN_LON, MAX_LON
# AREA_WIDTH
# IP_ADDRESS
PSNODE_BASE_PORT = int(os.getenv("PSNODE_BASE_PORT"))
VSNODE_BASE_PORT = int(os.getenv("VSNODE_BASE_PORT"))
PSNODE_NUM_PER_AREA = float(os.getenv("PSNODE_NUM_PER_AREA"))
AREA_PER_PSNODE_NUM = float(os.getenv("AREA_PER_PSNODE_NUM"))
MIN_LAT = float(os.getenv("MIN_LAT"))
MAX_LAT = float(os.getenv("MAX_LAT"))
MIN_LON = float(os.getenv("MIN_LON"))
MAX_LON = float(os.getenv("MAX_LON"))
AREA_WIDTH = float(os.getenv("AREA_WIDTH"))
IP_ADDRESS = os.getenv("IP_ADDRESS")
MEC_SERVER_NUM = int(os.getenv("MEC_SERVER_NUM"))

lineStep = AREA_WIDTH
forint = 1000

data = {"psnodes":[]}

# 始点となるArea
swLat = MIN_LAT
neLat = swLat + lineStep

# label情報
label_lat = 0
label_lon = 0

# ID用のindex
# id_index = (int(((MAX_LAT - MIN_LAT) * forint) * ((MAX_LON - MIN_LON) * forint))) * (MEC_SERVER_NUM - 1)
id_index = 0

# areaの数を数える
area_num = 0

# PNTypeをあらかじめ用意
pn_types = ["Temp_Sensor", "Humid_Sensor", "Anemometer"]
capabilities = {"Temp_Sensor":"Temperature", "Humid_Sensor":"Humidity", "Anemometer":"WindSpeed"}
vsnode_psnode_relation = {}
for i in range(len(pn_types)):
    vsnode_psnode_relation[pn_types[i]] = []

# PSNode/initial_environment.json の初期化
psnode_dir_path = os.getenv("PROJECT_PATH") + "/PSNode/"
initial_environment_file = psnode_dir_path + "initial_environment.json"
port_array = {
    "ports": []
}
with open(initial_environment_file, 'w') as file:
    json.dump(port_array, file, indent=4)

# VSNode/initial_environment.json の初期化
vsnode_dir_path = os.getenv("PROJECT_PATH") + "/VSNode/"
initial_environment_file = vsnode_dir_path + "initial_environment.json"
port_array = {
    "ports": []
}
with open(initial_environment_file, 'w') as file:
    json.dump(port_array, file, indent=4)

# 左下からスタートし，右へ進んでいく
# 端まで到達したら一段上へ
# 「エリアに何個ずつ」か「何エリアごとに1つ」かで場合分け
if PSNODE_NUM_PER_AREA > 0:
    while neLat <= MAX_LAT:
        swLon = MIN_LON
        neLon = swLon + lineStep
        label_lon = 0
        while neLon <= MAX_LON:
            i = 0
            while i < PSNODE_NUM_PER_AREA:
                data["psnodes"].append({"psnode":{}, "vsnode":{}})

                psnode_dict = generate_psnode_dict()
                data["psnodes"][-1]["psnode"] = psnode_dict

                vsnode_dict = generate_vsnode_dict()
                data["psnodes"][-1]["vsnode"] = vsnode_dict

                id_index += 1
                i += 1
            label_lon += 1
            swLon = ((swLon*forint) + (lineStep*forint)) / forint
            neLon = ((neLon*forint) + (lineStep*forint)) / forint
        label_lat += 1
        swLat = ((swLat*forint) + (lineStep*forint)) / forint
        neLat = ((neLat*forint) + (lineStep*forint)) / forint
elif AREA_PER_PSNODE_NUM > 0:
    while neLat <= MAX_LAT:
        swLon = MIN_LON
        neLon = swLon + lineStep
        label_lon = 0
        while neLon <= MAX_LON:
            if area_num % AREA_PER_PSNODE_NUM == 0:
                data["psnodes"].append({"psnode":{}, "vsnode":{}})

                psnode_dict = generate_psnode_dict()
                data["psnodes"][-1]["psnode"] = psnode_dict

                vsnode_dict = generate_vsnode_dict()
                data["psnodes"][-1]["vsnode"] = vsnode_dict

                id_index += 1
            label_lon += 1
            swLon = ((swLon*forint) + (lineStep*forint)) / forint
            neLon = ((neLon*forint) + (lineStep*forint)) / forint
        label_lat += 1
        swLat = ((swLat*forint) + (lineStep*forint)) / forint
        neLat = ((neLat*forint) + (lineStep*forint)) / forint

psnode_json = json_file_path + "config_main_psnode.json"
with open(psnode_json, 'w') as f:
    json.dump(data, f, indent=4)
