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
json_file = os.getenv("HOME") + os.getenv("PROJECT_NAME") + "/Main/config/json_files/config_main_psnode.json"
with open(json_file) as f:
    data = json.load(f)

for property in data["psnodes"]:
    # PSNode Data Property
    psnode = property["psnode"]
    label_psnode = psnode["property-label"]
    data_property_psnode = psnode["data-property"]
    node_psnode = Node(label_psnode, **data_property_psnode)
    node_psnode.add_label("PNode")
    global_graph.create(node_psnode)

    belonging_server_labal = psnode["relation-label"]["Server"]
    if belonging_server_labal == "S1":
        # Local GraphDB への登録
        dup_node_psnode = Node(label_psnode, **data_property_psnode)
        dup_node_psnode.add_label("PNode")
        local_graph.create(dup_node_psnode)
    
    # VSNode Data Property
    vsnode = property["vsnode"]
    label_vsnode = vsnode["property-label"]
    data_property_vsnode = vsnode["data-property"]
    node_vsnode = Node(label_vsnode, **data_property_vsnode)
    node_vsnode.add_label("VNode")
    global_graph.create(node_vsnode)
    if belonging_server_labal == "S1":
        # Local GraphDB への登録
        dup_node_vsnode = Node(label_vsnode, **data_property_vsnode)
        dup_node_vsnode.add_label("VNode")
        local_graph.create(dup_node_vsnode)
    
    # PSNode Object Property
    object_properties_psnode = psnode["object-property"]
    for object_property in object_properties_psnode:
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
        if belonging_server_labal == "S1":
            # Local GraphDB への登録
            dup_from_node = local_graph.nodes.match(from_node_label, **{from_node_property: from_node_value}).first()
            dup_to_node = local_graph.nodes.match(to_node_label, **{to_node_property: to_node_value}).first()
            dup_rel = Relationship(dup_from_node, rel_type, dup_to_node)
            local_graph.create(dup_rel)
    
    # VSNode Object Property
    object_properties_vpoint = vsnode["object-property"]
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
        if from_node_label == "VSNode" and to_node_label == "PSNode":
            # VSNode-VPoint のリレーション登録
            result_vpoint= global_graph.run("MATCH (n:PSNode), (m:PSink), (l:VPoint) WHERE n.Label = \"%s\" AND (n)-[:isConnectedTo]->(m)-[:isVirtualizedBy]->(l) RETURN l" % to_node_value)
            for record in result_vpoint:
                vpoint_vsnode_vpoint = record["l"]
                vpoint_vsnode_vpoint_label = vpoint_vsnode_vpoint["Label"]
            vpoint_vsnode_vsnode = global_graph.nodes.match(from_node_label, **{from_node_property: from_node_value}).first()
            rel_vsnode_vpoint = Relationship(vpoint_vsnode_vsnode, "isConnectedTo", vpoint_vsnode_vpoint)
            rel_vpoint_vsnode = Relationship(vpoint_vsnode_vpoint, "aggregates", vpoint_vsnode_vsnode)
            global_graph.create(rel_vsnode_vpoint)
            global_graph.create(rel_vpoint_vsnode)
        if belonging_server_labal == "S1":
            # Local GraphDB への登録
            dup_from_node = local_graph.nodes.match(from_node_label, **{from_node_property: from_node_value}).first()
            dup_to_node = local_graph.nodes.match(to_node_label, **{to_node_property: to_node_value}).first()
            dup_rel = Relationship(dup_from_node, rel_type, dup_to_node)
            local_graph.create(dup_rel)
            if from_node_label == "VSNode" and to_node_label == "PSNode":
                # VSNode-VPoint のリレーション登録
                local_result_vpoint= local_graph.run("MATCH (n:PSNode), (m:PSink), (l:VPoint) WHERE n.Label = \"%s\" AND (n)-[:isConnectedTo]->(m)-[:isVirtualizedBy]->(l) RETURN l" % to_node_value)
                for record in local_result_vpoint:
                    local_vpoint_vsnode_vpoint = record["l"]
                    local_vpoint_vsnode_vpoint_label = local_vpoint_vsnode_vpoint["Label"]
                local_vpoint_vsnode_vsnode = local_graph.nodes.match(from_node_label, **{from_node_property: from_node_value}).first()
                local_rel_vsnode_vpoint = Relationship(local_vpoint_vsnode_vsnode, "isConnectedTo", local_vpoint_vsnode_vpoint)
                local_rel_vpoint_vsnode = Relationship(local_vpoint_vsnode_vpoint, "aggregates", local_vpoint_vsnode_vsnode)
                local_graph.create(local_rel_vsnode_vpoint)
                local_graph.create(local_rel_vpoint_vsnode)
  
try:
    global_graph.commit(global_tx)
except:
    print("Cannot Register Data to Global GraphDB")
else:
    print("Success: PSNode and VSNode Instance in Global GraphDB")

#indexの付与
def create_index(graph, object, property):
    query = f"CREATE INDEX index_{object}_{property} IF NOT EXISTS FOR (n:{object}) ON (n.{property});"
    graph.run(query)

create_index(global_graph, "PSNode", "Label")
create_index(global_graph, "VSNode", "Label")


try:
    local_graph.commit(local_tx)
except:
    print("Cannot Register Data to Local GraphDB")
else:
    print("Success: PSNode and VSNode Instance in Local GraphDB")
    
#indexの付与
def create_index(graph, object, property):
    query = f"CREATE INDEX index_{object}_{property} IF NOT EXISTS FOR (n:{object}) ON (n.{property});"
    graph.run(query)

create_index(local_graph, "PSNode", "Label")
create_index(local_graph, "VSNode", "Label")