# !/bin/bash

# Mainプロセスで使用するconfigファイル (.json) を生成するためのスクリプトファイル
printf "\e[1;31m \n *** BUILD EXEC FILE *** \e[0m"

# ホーム下に用意した.envファイルを読み込む
source $HOME/.env

printf "\e[1;31m \n1. MAIN FRAMEWORK \e[0m"

printf "\e[1;31m \n1-1. BUILD MAIN PROCESS \e[0m"
# Mainプロセスの実行系ファイルを生成
go build -o ${PROJECT_PATH}/Main/main ${PROJECT_PATH}/Main/main.go

printf "\e[1;31m \n2. MEC SERVER FRAMEWORK \e[0m"

printf "\e[1;31m \n2-1. BUILD SERVER PROCESS \e[0m"
# MEC ServerのServerプロセスの実行系ファイルを生成
go build -o ${PROJECT_PATH}/MECServer/Server/main ${PROJECT_PATH}/MECServer/Server/main.go

printf "\e[1;31m \n2-2. BUILD VPOINT PROCESS \e[0m"
# MEC ServerのVPOINTプロセスの実行系ファイルを生成
go build -o ${PROJECT_PATH}/MECServer/VPoint/main ${PROJECT_PATH}/MECServer/VPoint/main.go

printf "\e[1;31m \n2-3. BUILD VSNODE PROCESS \e[0m"
# MEC ServerのVSNodeプロセスの実行系ファイルを生成
go build -o ${PROJECT_PATH}/MECServer/VSNode/main ${PROJECT_PATH}/MECServer/VSNode/main.go

printf "\e[1;31m \n3. PSNODE FRAMEWORK \e[0m"

printf "\e[1;31m \n3-1. BUILD PSNODE PROCESS \e[0m"
# PSNodeプロセスの実行系ファイルを生成
go build -o ${PROJECT_PATH}/PSNode/main ${PROJECT_PATH}/PSNode/main.go

printf "\n"