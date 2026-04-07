#!/bin/bash
# Lara Watchdog — logs incoming messages to a file
# Run in background: bash lara-watchdog.sh &
LOGFILE="$HOME/.lara/inbox.log"
SEEN_FILE="$HOME/.lara/seen_ids.txt"
touch "$SEEN_FILE" "$LOGFILE"

echo "$(date '+%Y-%m-%d %H:%M:%S') 🔥 Watchdog started" >> "$LOGFILE"

while true; do
    PORT=$(cat "$HOME/.lara/port" 2>/dev/null)
    if [ -z "$PORT" ] || [ "$PORT" = "0" ]; then
        sleep 60
        continue
    fi

    # Check all inbox keys for new messages
    CONTACTS=$(curl -s "http://localhost:$PORT/api/unread" -X POST \
        -H "Content-Type: application/json" -d '{"lastRead":{}}' 2>/dev/null)
    
    # Extract UIDs with unread counts > 0
    echo "$CONTACTS" | grep -oP '"([^"]{20})":\s*([1-9]\d*)' | while read -r line; do
        UID=$(echo "$line" | grep -oP '"[^"]{20}"' | tr -d '"')
        
        # Get messages from this contact
        MSGS=$(curl -s "http://localhost:$PORT/api/messages/$UID" 2>/dev/null)
        
        # Log new incoming messages we haven't seen
        echo "$MSGS" | grep -oP '"id":"([^"]*)"' | while read -r idline; do
            MSGID=$(echo "$idline" | cut -d'"' -f4)
            if ! grep -q "$MSGID" "$SEEN_FILE" 2>/dev/null; then
                # Get message text
                TEXT=$(echo "$MSGS" | grep -oP "\"id\":\"$MSGID\"[^}]*" | grep -oP '"text":"[^"]*"' | head -1 | cut -d'"' -f4)
                OUTGOING=$(echo "$MSGS" | grep -oP "\"id\":\"$MSGID\"[^}]*" | grep -oP '"outgoing":(true|false)' | head -1 | cut -d: -f2)
                
                if [ "$OUTGOING" = "false" ] && [ -n "$TEXT" ]; then
                    echo "$(date '+%Y-%m-%d %H:%M:%S') 📩 FROM=$UID TEXT=$TEXT" >> "$LOGFILE"
                fi
                echo "$MSGID" >> "$SEEN_FILE"
            fi
        done
    done
    
    sleep 60
done
