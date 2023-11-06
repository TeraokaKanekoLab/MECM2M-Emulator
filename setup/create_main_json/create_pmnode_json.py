import json
from dotenv import load_dotenv
import os
import math
import random
from datetime import datetime
import string
import ipaddress

# PMNode
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
## * Home IPv6 Address
## * Update Time
## * Velocity
## * Acceleration
## * Direction
# ---------------
## Object Property
## * aggregates (PSink->PNode)
## * isConnectedTo (PNode->PSink)
## * contains (PArea->PNode)
## * isInstalledIn (PNode->PArea)

# VMNode
# ---------------
## Data Property
## * VNodeID
## * Socket Address (VNode のソケットファイル，本来はIP:Port)
## * Software Module (VNode moduleの実行系ファイル)
## * Description
## * Representative Socket Address (VMNodeR のソケットファイル，本来はIP:Port)
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

def generate_pmnode_dict():
    # PMNode情報の追加
    parea_label = "PA" + str(label_lat) + ":" + str(label_lon)
    pmnode_label = "PMN" + str(id_index)
    pmnode_id = str(int(0b0100 << 60) + id_index)
    pnode_type = pn_types
    vnode_module = os.getenv("PROJECT_PATH") + "/VMNode/main"
    pnode_socket_address = ""
    pmnode_lat = random.uniform(swLat, neLat)
    pmnode_lon = random.uniform(swLon, neLon)
    capability = capabilities
    credential = "YES"
    session_key = generate_random_string(10)
    pmnode_description = "Description:" + pmnode_label
    home_ipv6_address = IP_ADDRESS
    update_time = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    velocity = 50.50
    acceleration = 10.10
    direction = "North"
    pmnode_dict = {
        "property-label": "PMNode",
        "data-property": {
            "Label": pmnode_label,
            "PNodeID": pmnode_id,
            "PNodeType": pnode_type,
            "VNodeModule": vnode_module,
            "SocketAddress": pnode_socket_address,
            "Position": [round(pmnode_lat, 4), round(pmnode_lon, 4)],
            "Capability": capability,
            "Credential": credential,
            "SessionKey": session_key,
            "Description": pmnode_description,
            "HomeIPv6Address": home_ipv6_address,
            "UpdateTime": update_time,
            "Velocity": velocity,
            "Acceleration": acceleration,
            "Direction": direction
        },
        "object-property": [
            {
                "from": {
                    "property-label": "PArea",
                    "data-property": "Label",
                    "value": parea_label
                },
                "to": {
                    "property-label": "PMNode",
                    "data-property": "Label",
                    "value": pmnode_label
                },
                "type": "contains"
            },
            {
                "from": {
                    "property-label": "PMNode",
                    "data-property": "Label",
                    "value": pmnode_label
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

    return pmnode_dict

def generate_vmnode_dict():
    # VMNode情報の追加
    parea_label = "PA" + str(label_lat) + ":" + str(label_lon)
    pmnode_label = "PMN" + str(id_index)
    vmnode_id = str(int(0b1100 << 60) + id_index)
    vmnode_port = VMNODE_BASE_PORT + id_index
    vnode_socket_address = IP_ADDRESS + ":" + str(vmnode_port)
    vmnoder_socket_address = "192.168.11.11" + ":" + str(VMNODER_BASE_PORT)
    vnode_module = os.getenv("PROJECT_PATH") + "/VMNode/main"
    vmnode_label = "VMN" + str(id_index)
    vmnode_description = "Description:" + vmnode_label
    vmnode_dict = {
        "property-label": "VMNode",
        "data-property": {
            "Label": vmnode_label,
            "VNodeID": vmnode_id,
            "SocketAddress": vnode_socket_address,
            "VMNodeRSocketAddress": vmnoder_socket_address,
            "SoftwareModule": vnode_module,
            "Description": vmnode_description
        },
        "object-property": [
            {
                "from": {
                    "property-label": "PMNode",
                    "data-property": "Label",
                    "value": pmnode_label
                },
                "to": {
                    "property-label": "VMNode",
                    "data-property": "Label",
                    "value": vmnode_label
                },
                "type": "isVirtualizedBy"
            },
            {
                "from": {
                    "property-label": "VMNode",
                    "data-property": "Label",
                    "value": vmnode_label
                },
                "to": {
                    "property-label": "PMNode",
                    "data-property": "Label",
                    "value": pmnode_label
                },
                "type": "isPhysicalizedBy"
            },
            {
                "from": {
                    "property-label": "VMNode",
                    "data-property": "Label",
                    "value": vmnode_label
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
                    "property-label": "VMNode",
                    "data-property": "Label",
                    "value": vmnode_label
                },
                "type": "contains"
            }
        ]
    }

    # VMNode/initial_environment.json に初期環境に配置されるVMNodeのポート番号を格納
    initial_environment_file = vmnode_dir_path + "initial_environment.json"
    with open(initial_environment_file, 'r') as file:
        ports_data = json.load(file)
    ports_data["ports"].append(vmnode_port)
    with open(initial_environment_file, 'w') as file:
        json.dump(ports_data, file, indent=4)

    return vmnode_dict

load_dotenv()
json_file_path = os.getenv("PROJECT_PATH") + "/setup/GraphDB/config/"

# VMNODEH_BASE_PORT
# PSINK_NUM_PER_AREA
# MIN_LAT, MAX_LAT, MIN_LON, MAX_LON
# AREA_WIDTH
# IP_ADDRESS
VMNODE_BASE_PORT = int(os.getenv("VMNODE_BASE_PORT"))
VMNODER_BASE_PORT = int(os.getenv("VMNODER_BASE_PORT"))
PSNODE_NUM_PER_AREA = float(os.getenv("PSNODE_NUM_PER_AREA"))
AREA_PER_PSNODE_NUM = float(os.getenv("AREA_PER_PSNODE_NUM"))
MIN_LAT = float(os.getenv("MIN_LAT"))
MAX_LAT = float(os.getenv("MAX_LAT"))
MIN_LON = float(os.getenv("MIN_LON"))
MAX_LON = float(os.getenv("MAX_LON"))
AREA_WIDTH = float(os.getenv("AREA_WIDTH"))
IP_ADDRESS = os.getenv("IP_ADDRESS")

lineStep = AREA_WIDTH
forint = 1000

data = {"pmnodes":[]}

# 始点となるArea
swLat = MIN_LAT
neLat = swLat + lineStep

# label情報
label_lat = 0
label_lon = 0

# ID用のindex
id_index = 0

# areaの数を数える
area_num = 0

# PNTypeをあらかじめ用意
pn_types = ["AirFlowMeter",
            "VacuumSensor",
            "O2Sensor",
            "AFSensor",
            "SlotPositionSensor",
            "CrankPositionSensor",
            "CamPositionSensor",
            "TemperatureSensorForEngineControl",
            "KnockSensor",
            "AccelPositionSensor",
            "SteeringSensor",
            "HeightControlSensor",
            "WheelSpeedSensor",
            "YawRateSensor",
            "OilTemperatureSensor",
            "TorqueSensorForElectronicPowerSteering",
            "AirBagSensor",
            "UltrasonicSensor",
            "TirePressureSensor",
            "RadarSensor",
            "SensingCamera",
            "TouchSensor",
            "AutoAirConditionerSensor",
            "AutoRightSensor",
            "FuelSensor",
            "RainSensor",
            "AirQualitySensor",
            "GyroSensor",
            "AlcoholInterlockSensor"]
capabilities = ["AFMAirIntakeAmount",
            "VSAirIntakeAmount",
            "O2SOxygenConcentration",
            "AFSOxygenConcentration",
            "SPSAccelPosition",
            "CrPSEngineRPM",
            "CaPSEngineRPM",
            "TemperatureEC",
            "Knocking",
            "APSAccelPosition",
            "HandleAngle",
            "HCSDistance",
            "WheelSpeed",
            "RotationalSpeed",
            "OTemperature",
            "TorquePower",
            "AirBag",
            "Hz",
            "kPa",
            "RSDistance",
            "Frame",
            "TouchPosition",
            "AASTemperature",
            "Lux",
            "Gallon",
            "Milli",
            "Gass",
            "Direction",
            "Alcohol"]

# VMNode/initial_environment.json の初期化
vmnode_dir_path = os.getenv("PROJECT_PATH") + "/VMNode/"
initial_environment_file = vmnode_dir_path + "initial_environment.json"
port_array = {
    "ports": []
}
with open(initial_environment_file, 'w') as file:
    json.dump(port_array, file, indent=4)


#左下からスタートし，右へ進んでいく
#端まで到達したら一段上へ
# 「エリアに何個ずつ」か「何エリアごとに1つ」かで場合分け
if PSNODE_NUM_PER_AREA > 0:
    while neLat <= MAX_LAT:
        swLon = MIN_LON
        neLon = swLon + lineStep
        label_lon = 0
        while neLon <= MAX_LON:
            i = 0
            while i < PSNODE_NUM_PER_AREA:
                data["pmnodes"].append({"pmnode":{}, "vmnode":{}})

                pmnode_dict = generate_pmnode_dict()
                data["pmnodes"][-1]["pmnode"] = pmnode_dict

                vmnode_dict = generate_vmnode_dict()
                data["pmnodes"][-1]["vmnode"] = vmnode_dict

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
                data["pmnodes"].append({"pmnode":{}, "vmnode":{}})

                pmnode_dict = generate_pmnode_dict()
                data["pmnodes"][-1]["pmnode"] = pmnode_dict

                vmnode_dict = generate_vmnode_dict()
                data["pmnodes"][-1]["vmnode"] = vmnode_dict

                id_index += 1
            area_num += 1
            label_lon += 1
            swLon = ((swLon*forint) + (lineStep*forint)) / forint
            neLon = ((neLon*forint) + (lineStep*forint)) / forint
        label_lat += 1
        swLat = ((swLat*forint) + (lineStep*forint)) / forint
        neLat = ((neLat*forint) + (lineStep*forint)) / forint


pmnode_json = json_file_path + "/config_main_pmnode.json"
with open(pmnode_json, 'w') as f:
    json.dump(data, f, indent=4)