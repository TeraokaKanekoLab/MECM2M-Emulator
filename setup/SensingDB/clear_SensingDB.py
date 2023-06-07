import pymysql
from dotenv import load_dotenv
import os

load_dotenv()

host = "localhost"
user = os.getenv("MYSQL_USERNAME")
password = os.getenv("MYSQL_PASSWORD")
database = os.getenv("MYSQL_LOCAL_DB")
table = os.getenv("MYSQL_TABLE")

# DBに接続
connection = pymysql.connect(host=host, user=user, password=password, database=database)
cursor = connection.cursor()

# テーブル内の全データを削除するクエリを実行
delete_query = f"DELETE FROM {table};"
cursor.execute(delete_query)

# テーブルを削除する
drop_table_query = f"DROP TABLE {table}"
cursor.execute(drop_table_query)

# 変更をコミットして，接続を切断する
connection.commit()
cursor.close()
connection.close()

print("Clear SensingDB")