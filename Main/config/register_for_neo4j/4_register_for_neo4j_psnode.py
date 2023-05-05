import json
from py2neo import Graph, Node, Relationship
import sys
from dotenv import load_dotenv
import os

load_dotenv()
url = "bolt://localhost:" + os.getenv("NEO4J_PORT")
graph = Graph(url, auth=(os.getenv("NEO4J_USERNAME"), os.getenv("NEO4J_PASSWORD")))

tx = graph.begin()

args = sys.argv
json_file = os.getenv("PROJECT_PATH") + "/Main/config/json_files/config_main_psnode.json"
with open(json_file) as f:
    data = json.load(f)

psnode = data["psnodes"]["psnode"]
vsnode = data["psnodes"]["vsnode"]

for psn in psnode:
    label = psn["property-label"]
    relation = psn["relation-label"]
    data_property = psn["data-property"]
    node = Node(label, **data_property)
    node.add_label("PNode")
    graph.create(node)
    object_properties = psn["object-property"]
    for object_property in object_properties:
        from_node_label = object_property["from"]["property-label"]
        from_node_property = object_property["from"]["data-property"]
        from_node_value = object_property["from"]["value"]
        to_node_label = object_property["to"]["property-label"]
        to_node_property = object_property["to"]["data-property"]
        to_node_value = object_property["to"]["value"]
        rel_type = object_property["type"]
        from_node = graph.nodes.match(from_node_label, **{from_node_property: from_node_value}).first()
        to_node = graph.nodes.match(to_node_label, **{to_node_property: to_node_value}).first()
        rel = Relationship(from_node, rel_type, to_node)
        graph.create(rel)

for vsn in vsnode:
    label = vsn["property-label"]
    data_property = vsn["data-property"]
    node = Node(label, **data_property)
    node.add_label("VNode")
    graph.create(node)
    #vsnode-vpointのリレーションを作成 (start)
    #vsnode-vpointのリレーションを作成 (end)
    object_properties = psn["object-property"]
    for object_property in object_properties:
        from_node_label = object_property["from"]["property-label"]
        from_node_property = object_property["from"]["data-property"]
        from_node_value = object_property["from"]["value"]
        to_node_label = object_property["to"]["property-label"]
        to_node_property = object_property["to"]["data-property"]
        to_node_value = object_property["to"]["value"]
        rel_type = object_property["type"]
        from_node = graph.nodes.match(from_node_label, **{from_node_property: from_node_value}).first()
        to_node = graph.nodes.match(to_node_label, **{to_node_property: to_node_value}).first()
        rel = Relationship(from_node, rel_type, to_node)
        graph.create(rel)
        if from_node_label == "VSNode" and to_node_label == "PSNode":
            #vsnode-vpointのリレーションを作成 (start)
            result_vpoint= graph.run("MATCH (n:PSNode), (m:PSink), (l:VPoint) WHERE n.label = \"%s\" AND (n)-[:respondsViaDevApi]->(m)-[:isVirtualizedWith]->(l) return l" % to_node_value)
            for record in result_vpoint:
                vpoint_psnode_vpoint = record["l"]
                vpoint_psnode_vpoint_label = vpoint_psnode_vpoint["Label"]
            rel_vsnode_vpoint = Relationship(from_node, "requestsViaPrimApi", vpoint_psnode_vpoint)
            rel_vpoint_vsnode = Relationship(vpoint_psnode_vpoint, "respondsViaPrimApi", from_node)
            graph.create(rel_vsnode_vpoint)
            graph.create(rel_vpoint_vsnode)
            #vsnode-vpointのリレーションを作成 (end)
            #vsnode-serverのリレーションを作成 (start)
            result_server = graph.run("MATCH (n:VPoint), (m:Server) WHERE n.label = \"%s\" AND (n)-[:isRunningOn]->(m) return m" % vpoint_psnode_vpoint_label)
            for record in result_server:
                vsnode_server_server = record["m"]
            rel_vsnode_server = Relationship(from_node, "isRunningOn", vsnode_server_server)
            rel_server_vsnode = Relationship(vsnode_server_server, "supports", from_node)
            graph.create(rel_vsnode_server)
            graph.create(rel_server_vsnode)
            #vsnode-serverのリレーションを作成 (end)

try:
    graph.commit(tx)
except:
    print("Cannot Register Data to Neo4j")
else:
    print("Success: PSNode and VSNode Instance")

#indexの付与
def create_index(graph, object, property):
    query = f"CREATE INDEX index_{object}_{property} IF NOT EXISTS FOR (n:{object}) ON (n.{property});"
    graph.run(query)

create_index(graph, "PSNode", "Label")
create_index(graph, "VSNode", "Label")



"""
for property in data["psnodes"]:
    for physical in property["psnode"]:
        label = physical["property-label"]
        relation = physical["relation-label"]
        data_property = physical["data-property"]
        node = Node(label, **data_property)
        node.add_label("PNode")
        graph.create(node)
        #psnode-vpointのリレーションを作成 (start)
        psink = relation["PSink"]
        pnode_id = relation["PNodeID"]
        config_file = relation["config-file"]
        result = graph.run("MATCH (n:PSink), (m:VPoint) WHERE n.label = \"%s\" AND (n)-[:isVirtualizedWith]->(m) return m" % psink)
        for record in result:
            vpoint_psnode_vpoint = record["m"]
            vpoint_psnode_vpoint_label = vpoint_psnode_vpoint["label"]
        rel_psnode_vpoint = Relationship(node, "isVirtualizedWith", vpoint_psnode_vpoint)
        rel_vpoint_psnode = Relationship(vpoint_psnode_vpoint, "isComposedOf", node)
        graph.create(rel_psnode_vpoint)
        graph.create(rel_vpoint_psnode)
        #psnode-vpointのリレーションを作成 (end)
        object_properties = physical["object-property"]
        for object_property in object_properties:
            from_node_label = object_property["from"]["property-label"]
            from_node_property = object_property["from"]["data-property"]
            from_node_value = object_property["from"]["value"]
            to_node_label = object_property["to"]["property-label"]
            to_node_property = object_property["to"]["data-property"]
            to_node_value = object_property["to"]["value"]
            rel_type = object_property["type"]
            from_node = graph.nodes.match(from_node_label, **{from_node_property: from_node_value}).first()
            to_node = graph.nodes.match(to_node_label, **{to_node_property: to_node_value}).first()
            rel = Relationship(from_node, rel_type, to_node)
            graph.create(rel)
    for virtual in property["vsnode"]:
        label = virtual["property-label"]
        data_property = virtual["data-property"]
        node = Node(label, **data_property)
        node.add_label("VNode")
        graph.create(node)
        #vsnode-vpointのリレーションを作成 (start)
        rel_vsnode_vpoint = Relationship(node, "requestsViaPrimApi", vpoint_psnode_vpoint)
        rel_vpoint_vsnode = Relationship(vpoint_psnode_vpoint, "respondsViaPrimApi", node)
        graph.create(rel_vsnode_vpoint)
        graph.create(rel_vpoint_vsnode)
        #vsnode-vpointのリレーションを作成 (end)
        #vsnode-serverのリレーションを作成 (start)
        result = graph.run("MATCH (n:VPoint), (m:Server) WHERE n.label = \"%s\" AND (n)-[:isRunningOn]->(m) return m" % vpoint_psnode_vpoint_label)
        for record in result:
            vsnode_server_server = record["m"]
        rel_vsnode_server = Relationship(node, "isRunningOn", vsnode_server_server)
        rel_server_vsnode = Relationship(vsnode_server_server, "supports", node)
        graph.create(rel_vsnode_server)
        graph.create(rel_server_vsnode)
        #vsnode-serverのリレーションを作成 (end)
        object_properties = virtual["object-property"]
        for object_property in object_properties:
            from_node_label = object_property["from"]["property-label"]
            from_node_property = object_property["from"]["data-property"]
            from_node_value = object_property["from"]["value"]
            to_node_label = object_property["to"]["property-label"]
            to_node_property = object_property["to"]["data-property"]
            to_node_value = object_property["to"]["value"]
            rel_type = object_property["type"]
            from_node = graph.nodes.match(from_node_label, **{from_node_property: from_node_value}).first()
            to_node = graph.nodes.match(to_node_label, **{to_node_property: to_node_value}).first()
            rel = Relationship(from_node, rel_type, to_node)
            graph.create(rel)

graph.commit(tx)
"""    