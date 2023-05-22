import json
from py2neo import Graph, Node, Relationship
import sys
from dotenv import load_dotenv
import os

load_dotenv()
global_url = "bolt://localhost:" + os.getenv("NEO4J_GLOBAL_PORT_PYTHON")
global_graph = Graph(global_url, auth=(os.getenv("NEO4J_USERNAME"), os.getenv("NEO4J_GLOBAL_PASSWORD")))
global_tx = global_graph.begin()

local_url = "bolt://localhost:" + os.getenv("NEO4J_LOCAL_PORT_PYTHON")
local_graph = Graph(local_url, auth=(os.getenv("NEO4J_USERNAME"), os.getenv("NEO4J_LOCAL_PASSWORD")))
local_tx = local_graph.begin()

args = sys.argv
json_file = os.getenv("HOME") + os.getenv("PROJECT_NAME") + "/Main/config/json_files/config_main_psink.json"
with open(json_file) as f:
    data = json.load(f)

for property in data["psinks"]:
    # PSink Data Property
    psink = property["psink"]
    label_psink = psink["property-label"]
    data_property_psink = psink["data-property"]
    node_psink = Node(label_psink, **data_property_psink)
    global_graph.create(node_psink)

    belonging_server_label = psink["relation-label"]["Server"]
    if belonging_server_label == "S1":
        # Local GraphDB への登録
        dup_node_psink = Node(label_psink, **data_property_psink)
        local_graph.create(dup_node_psink)
    
    # VPoint Data Property
    vpoint = property["vpoint"]
    label_vpoint = vpoint["property-label"]
    data_property_vpoint = vpoint["data-property"]
    node_vpoint = Node(label_vpoint, **data_property_vpoint)
    global_graph.create(node_vpoint)
    if belonging_server_label == "S1":
        # Local GraphDB への登録
        dup_node_vpoint = Node(label_vpoint, **data_property_vpoint)
        local_graph.create(dup_node_vpoint)

    # PSink Object Property
    object_properties_psink = psink["object-property"]
    for object_property in object_properties_psink:
        from_node_label = object_property["from"]["property-label"]
        from_node_property = object_property["from"]["data-property"]
        from_node_value = object_property["from"]["value"]
        to_node_label = object_property["to"]["property-label"]
        to_node_property = object_property["to"]["data-property"]
        to_node_value = object_property["to"]["value"]
        rel_type = object_property["type"]
        from_node = global_graph.nodes.match(from_node_label, **{from_node_property: from_node_value}).first()
        to_node = global_graph.nodes.match(to_node_label, **{to_node_property: to_node_value}).first()
        rel = Relationship(from_node, rel_type, to_node)
        global_graph.create(rel)
        if belonging_server_label == "S1":
            # Local GraphDB への登録
            dup_from_node = local_graph.nodes.match(from_node_label, **{from_node_property: from_node_value}).first()
            dup_to_node = local_graph.nodes.match(to_node_label, **{to_node_property: to_node_value}).first()
            dup_rel = Relationship(dup_from_node, rel_type, dup_to_node)
            local_graph.create(dup_rel)

    # VPoint Object Property
    object_properties_vpoint = vpoint["object-property"]
    for object_property in object_properties_vpoint:
        from_node_label = object_property["from"]["property-label"]
        from_node_property = object_property["from"]["data-property"]
        from_node_value = object_property["from"]["value"]
        to_node_label = object_property["to"]["property-label"]
        to_node_property = object_property["to"]["data-property"]
        to_node_value = object_property["to"]["value"]
        rel_type = object_property["type"]
        from_node = global_graph.nodes.match(from_node_label, **{from_node_property: from_node_value}).first()
        to_node = global_graph.nodes.match(to_node_label, **{to_node_property: to_node_value}).first()
        rel = Relationship(from_node, rel_type, to_node)
        global_graph.create(rel)
        if belonging_server_label == "S1":
            # Local GraphDB への登録
            dup_from_node = local_graph.nodes.match(from_node_label, **{from_node_property: from_node_value}).first()
            dup_to_node = local_graph.nodes.match(to_node_label, **{to_node_property: to_node_value}).first()
            dup_rel = Relationship(dup_from_node, rel_type, dup_to_node)
            local_graph.create(dup_rel)

try:
    global_graph.commit(global_tx)
except:
    print("Cannot Register Data to Global GraphDB")
else:
    print("Success: PSink and VPoint Instance in Global GraphDB")
    
#indexの付与
def create_index(graph, object, property):
    query = f"CREATE INDEX index_{object}_{property} IF NOT EXISTS FOR (n:{object}) ON (n.{property});"
    graph.run(query)

create_index(global_graph, "PSink", "Label")
create_index(global_graph, "VPoint", "Label")


try:
    local_graph.commit(local_tx)
except:
    print("Cannot Register Data to Local GraphDB")
else:
    print("Success: PSink and VPoint Instance in Local GraphDB")
    
#indexの付与
def create_index(graph, object, property):
    query = f"CREATE INDEX index_{object}_{property} IF NOT EXISTS FOR (n:{object}) ON (n.{property});"
    graph.run(query)

create_index(local_graph, "PSink", "Label")
create_index(local_graph, "VPoint", "Label")