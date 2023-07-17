import json
from dotenv import load_dotenv
import os
import random
import math

load_dotenv()
json_file_path = os.getenv("PROJECT_PATH") + "/LinkProcess/ClosedNetwork"

# EDGE_SERVER_NUM
EDGE_SERVER_NUM = int(os.getenv("EDGE_SERVER_NUM"))
LINK_SOCKET_ADDRESS_ROOT = "/tmp/mecm2m/link-process/"

# EDGE_SERVER_NUM 分だけファイルが生成される

# はじめに，元のデータを削除しておく
i = 1
while i <= EDGE_SERVER_NUM:
    full_path = json_file_path + "/closed_network_link_process_" + str(i) + ".json"
    with open(full_path, 'w') as f:
        json.dump({"socket_addresses":[]}, f)
    i += 1

psnode_config_file = os.getenv("PROJECT_PATH") + "/Main/config/json_files/config_main_psnode.json"
with open(psnode_config_file) as file:
    psnodes = json.load(file)
    psnodes_array = psnodes['psnodes']
    for psnode in psnodes_array:
        psnode_data_property = psnode["psnode"]["data-property"]
        pnode_id = psnode_data_property["PNodeID"]
        socket_address = psnode_data_property["SocketAddress"]
        # ソケットアドレスからサーバ番号とVNodeIDを抽出
        server_num_index = socket_address.find("_")
        vnode_id_index = socket_address.rfind("_")
        dot_index = socket_address.rfind(".")
        server_num = socket_address[server_num_index+1:vnode_id_index]
        vnode_id = socket_address[vnode_id_index+1:dot_index]
        # サーバ番号に応じて，対応するソケットアドレス格納ファイル (json) に追加していく
        i = 1
        while i <= EDGE_SERVER_NUM:
            if str(i) == server_num:
                link_socket_addr = LINK_SOCKET_ADDRESS_ROOT + "closed-network_" + vnode_id + "_" + pnode_id + ".sock"
                full_path = json_file_path + "/closed_network_link_process_" + server_num + ".json"
                with open(full_path, 'r') as f:
                    socket_file_data = json.load(f)
                socket_file_data["socket_addresses"].append(link_socket_addr)
                with open(full_path, 'w') as f:
                    json.dump(socket_file_data, f, indent=4)
            i += 1