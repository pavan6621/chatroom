package main

import (
	"context"
	"encoding/json"

	//"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	//"math/rand"
)

type Message struct {
	Sender   string `json:"sender"`
	Content  string `json:"content"`
	RoomName string `json:"roomname"`
	Time     string `json:"time,omitempty"`
	//Date     time.Time  `json:"date,omiempty"`
	RecipientID string `json:"recipientid"`
	Type        string `json:"type"`
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096, // Increased read buffer size to 4096 bytes
	WriteBufferSize: 4096, // Increased write buffer size to 4096 bytes
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins
	},
}

type Client struct {
	id   string
	conn *websocket.Conn
	send chan []byte
	room *Room
}

type Room struct {
	name       string
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
	mu         sync.Mutex // Mutex for synchronization
}

type Hub struct {
	rooms map[string]*Room
	mu    sync.Mutex
}

var (
	hub         = Hub{rooms: make(map[string]*Room)}
	mongoClient *mongo.Client
	ctx         = context.TODO()
	roomsCol    *mongo.Collection
	clientsCol  *mongo.Collection
	roomsCol2   *mongo.Collection
	clientsCol2 *mongo.Collection
)

func initMongoDB() {
	var err error
	mongoClient, err = mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	roomsCol = mongoClient.Database("chat_app").Collection("rooms")
	clientsCol = mongoClient.Database("chat_app").Collection("clients")
	roomsCol2 = mongoClient.Database("chat_app").Collection("specificrooms")
	clientsCol2 = mongoClient.Database("chat_app").Collection("specificclients")

}

func NewRoom(name string) *Room {
	return &Room{
		name:       name,
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte),
	}
}

func (r *Room) Run() {
	log.Printf("Room '%s' created", r.name)
	// defer func() {
	// 	r.mu.Lock()
	// 	delete(hub.rooms, r.name)
	// 	r.mu.Unlock()

	// 	_, err := roomsCol.DeleteOne(ctx, bson.M{"name": r.name})
	// 	if err != nil {
	// 		log.Printf("Failed to delete room '%s' from MongoDB: %v", r.name, err)
	// 	} else {
	// 		log.Printf("Room '%s' deleted from MongoDB", r.name)
	// 	}
	// }()

	for {
		select {
		case client := <-r.register:
			r.mu.Lock()
			r.clients[client] = true
			r.mu.Unlock()
			log.Printf("User '%s' joined room '%s'", client.id, r.name)
		case client := <-r.unregister:
			r.mu.Lock()
			if _, ok := r.clients[client]; ok {
				delete(r.clients, client)
				close(client.send)
				log.Printf("User '%s' disconnected from room '%s'", client.id, r.name)
				go func() {
					_, err := clientsCol.DeleteOne(ctx, bson.M{"room": r.name, "client_id": client.id})
					if err != nil {
						log.Printf("Failed to delete client from MongoDB: %v", err)
					}
				}()
			}

		case msg := <-r.broadcast:
			r.mu.Lock()

			// Broadcast the incoming message to connected clients
			for client := range r.clients {
				select {
				case client.send <- []byte(msg): // Assuming msg is already a JSON string
				// default:
				// 	close(client.send)
				// 	delete(r.clients, client)
				default:
				}
			}

			r.mu.Unlock()
		}
	}
}

func (c *Client) readPump() {
	log.Printf("readpump ####")
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			break
		}
		log.Printf("raw message recieved is:%s", string(message))
		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Error unmarshalling message: %v", err)
			continue
		}

		log.Printf("Content: %s, Sender: %s, Room Name: %s, receipeint id: %s", msg.Content, msg.Sender, msg.RoomName, msg.RecipientID)
		parts := strings.SplitN(string(msg.Content), ":", 3)
		log.Printf("Parts after splitting: %v", parts)
		//var recipientID=""
		//var recipientID, msgContent string
		// if len(parts) == 3 && parts[0] == "to" {
		//     recipientID = parts[1]
		// 	msg.Content=parts[2]
		// 	log.Printf("content is: %s",parts[2])
		// 	log.Printf("part 1 is:%v",parts[1])
		//     //msgContent = parts[2]
		// } else {
		//     recipientID = "all"

		//     //msgContent = string(message)
		// }
		log.Printf("recipientid is: %s", msg.RecipientID)

		msg.Time = time.Now().Format("02-01-2006T03:04:05.000PM07:00")

		// Save the message to MongoDB
		_, err = mongoClient.Database("chat_app").Collection("recievemessages").InsertOne(context.TODO(), bson.M{
			"roomname": msg.RoomName,
			"sender":   msg.Sender,
			"content":  msg.Content,
			// "clientID":    clientID,
			"recipientid": msg.RecipientID,
			"type":        msg.Type,
			"time":        msg.Time,
		})
		if err != nil {
			log.Printf("Failed to insert message into MongoDB: %v", err)
			continue
		}

		jsonMessage, err := json.Marshal(bson.M{
			"Room":    msg.RoomName,
			"Sender":  msg.Sender,
			"Content": msg.Content,
			// "clientID":    clientID,
			"RecipientID": msg.RecipientID,
			"Type":        msg.Type,
			"Time":        msg.Time,
		})
		if err != nil {
			log.Printf("Error marshalling message: %v", err)
			continue
		}

		if msg.RecipientID != "all" {
			// for client := range c.room.clients {
			//     if client.id == msg.RecipientID {
			//         log.Printf("Recipient found: %s", client.id)

			//         // Add a check to ensure the WebSocket connection is open
			//         if client.conn != nil {
			//             log.Printf("Sending message to client ID: %s", client.id)

			//             // select {
			//              client.send <- jsonMessage
			//                 log.Printf("Message sent to client ID: %s", jsonMessage)
			//             // default:
			//             //     log.Printf("Send channel for client ID %s is full or not being read from", client.id)
			//             // }
			//         } else {
			//             log.Printf("WebSocket connection for client ID %s is nil", client.id)
			//         }

			//         break
			//     }
			// }
			//createRoomAndAddUsers(c, msg.RecipientID, jsonMessage)

		} else {
			// Broadcasting to all clients
			// for client := range c.room.clients {
			//     log.Printf("Broadcasting message to client ID: %s", client.id)

			//     select {
			//     case client.send <- jsonMessage:
			//         log.Printf("Message sent to client ID: %s", client.id)
			//     default:
			//         log.Printf("Send channel for client ID %s is full or not being read from", client.id)
			//     }
			// }

			broadcastToRoom(c.room, jsonMessage)

		}

	}

}

//	func generateRandomRoomName() string {
//		rand.Seed(time.Now().UnixNano())
//		chars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
//		length := 8
//		var roomName strings.Builder
//		for i := 0; i < length; i++ {
//			roomName.WriteByte(chars[rand.Intn(len(chars))])
//		}
//		return roomName.String()
//	}
func (c *Client) writePump() {
	//defer c.conn.Close()
	log.Printf("writepump ####")
	for message := range c.send {
		err := c.conn.WriteMessage(websocket.TextMessage, message)
		log.Printf("message to write:%s", message)
		if err != nil {
			log.Println(err)
			break
		}
	}
}
func createRoomAndAddUsers(c *gin.Context) {
	roomName := c.Query("room_name")
	senderID := c.Query("sender_id")
	clientID := c.Query("recipient_id")

	// Check if both users are present in the same room
	var senderDocs []bson.M
	cursor, err := clientsCol2.Find(ctx, bson.M{"client_id": senderID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query MongoDB for sender"})
		return
	}
	if err = cursor.All(ctx, &senderDocs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode sender documents"})
		return
	}

	var recipientDocs []bson.M
	cursor, err = clientsCol2.Find(ctx, bson.M{"client_id": clientID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query MongoDB for recipient"})
		return
	}
	if err = cursor.All(ctx, &recipientDocs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode recipient documents"})
		return
	}

	// Check if both users are present in the same room
	foundCommonRoom := false
	var commonRoom string
	for _, sender := range senderDocs {
		senderRoomName := sender["room"].(string)
		for _, recipient := range recipientDocs {
			recipientRoomName := recipient["room"].(string)
			if senderRoomName == recipientRoomName {
				foundCommonRoom = true
				commonRoom = senderRoomName
				break
			}
		}
		if foundCommonRoom {
			break
		}
	}

	if foundCommonRoom {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Room already exists with both users", "room": commonRoom})
		return
	}

	hub.mu.Lock()
	defer hub.mu.Unlock()

	// Check if the room already exists in the hub
	room, ok := hub.rooms[roomName]
	if !ok {
		// Room doesn't exist in the hub, create a new one
		room = NewRoom(roomName)
		hub.rooms[roomName] = room
		go room.Run()

		// Insert the room into MongoDB
		_, err := roomsCol2.InsertOne(ctx, bson.M{"name": roomName})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to insert room into MongoDB"})
			return
		}
	}

	// Register sender
	senderClient := &Client{
		id:   senderID,
		room: room,
		send: make(chan []byte, 256),
	}
	room.register <- senderClient

	// Insert the sender into MongoDB
	_, err = clientsCol2.InsertOne(ctx, bson.M{"room": roomName, "client_id": senderID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to insert sender into MongoDB"})
		return
	}

	// Register client
	client := &Client{
		id:   clientID,
		room: room,
		send: make(chan []byte, 256),
	}
	room.register <- client

	// Insert the client into MongoDB
	_, err = clientsCol2.InsertOne(ctx, bson.M{"room": roomName, "client_id": clientID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to insert client into MongoDB"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"room": roomName})
}

func broadcastToRoom(room *Room, message []byte) {
	log.Printf("broadcastmessage:")
	for client := range room.clients {
		log.Printf("Broadcasting message to client ID: %s", client.id)
		select {
		case client.send <- message:
			log.Printf("Message sent to client ID: %s", client.id)
		default:
			log.Printf("Send channel for client ID %s is full or not being read from", client.id)
		}
	}
}

func createRoomAndUser(c *gin.Context) {
	log.Printf("createromm ####")
	roomName := c.Query("room")
	if roomName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Room name is required"})
		return
	}

	clientID := c.Query("id")
	if clientID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Client ID is required"})
		return
	}

	hub.mu.Lock()
	defer hub.mu.Unlock()

	// Check if the room already exists in the hub
	room, ok := hub.rooms[roomName]
	if !ok {
		// Room doesn't exist in the hub, create a new one
		room = NewRoom(roomName)
		hub.rooms[roomName] = room
		go room.Run()

		// Insert the room into MongoDB if it doesn't exist
		_, err := roomsCol.InsertOne(ctx, bson.M{"name": roomName})
		if err != nil {
			log.Printf("Failed to insert room into MongoDB: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create room"})
			return
		}
	}

	// Check if the client already exists in the database
	var existingClient bson.M
	err := clientsCol.FindOne(ctx, bson.M{"room": roomName, "client_id": clientID}).Decode(&existingClient)
	if err != nil && err != mongo.ErrNoDocuments {
		log.Printf("Failed to query MongoDB: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to verify client"})
		return
	}
	if existingClient != nil {
		// Client already exists in the database
		c.JSON(http.StatusBadRequest, gin.H{"message": "User already registered in the room"})
		return
	}

	// Client doesn't exist in the room, register the client
	client := &Client{
		id:   clientID,
		room: room,
		send: make(chan []byte, 256),
	}
	room.register <- client

	// Insert the client into MongoDB
	_, err = clientsCol.InsertOne(ctx, bson.M{"room": roomName, "client_id": clientID})
	if err != nil {
		log.Printf("Failed to insert client into MongoDB: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register client"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Room and user created/registered successfully"})
}

func serveWs(c *gin.Context) {
	log.Printf("#####")
	roomName := c.Query("room")
	if roomName == "" {
		log.Printf("Room name is required")
		return
	}

	clientID := c.Query("id")
	if clientID == "" {
		log.Printf("Client ID is required")
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection to WebSocket: %v", err)
		return
	}

	hub.mu.Lock()
	room, ok := hub.rooms[roomName]
	if !ok {
		log.Printf("Room '%s' not found", roomName)
		//conn.Close()
		hub.mu.Unlock()
		return
	}
	hub.mu.Unlock()

	// clientExists := false
	// for client := range room.clients {
	// 	if client.id == clientID {
	// 		clientExists = true
	// 		break
	// 	}
	// }

	// if !clientExists {
	// 	log.Printf("User '%s' not found in room '%s'", clientID, roomName)
	// 	conn.Close()
	// 	return
	// }

	client := &Client{
		id:   clientID,
		conn: conn,
		send: make(chan []byte, 256),
		room: room,
	}

	room.register <- client

	go client.writePump()
	go client.readPump()
}

func listRooms(c *gin.Context) {
	log.Printf("listroom @@@@@")
	cursor, err := roomsCol.Find(ctx, bson.M{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve rooms"})
		return
	}
	defer cursor.Close(ctx)

	var rooms []string
	for cursor.Next(ctx) {
		var room bson.M
		if err := cursor.Decode(&room); err != nil {
			log.Printf("Failed to decode room: %v", err)
			continue
		}
		rooms = append(rooms, room["name"].(string))
	}

	c.JSON(http.StatusOK, gin.H{"rooms": rooms})
}
func listSpecificRooms(c *gin.Context) {
	log.Printf("listroom @@@@@")
	cursor, err := roomsCol2.Find(ctx, bson.M{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve rooms"})
		return
	}
	defer cursor.Close(ctx)

	var rooms []string
	for cursor.Next(ctx) {
		var room bson.M
		if err := cursor.Decode(&room); err != nil {
			log.Printf("Failed to decode room: %v", err)
			continue
		}
		rooms = append(rooms, room["name"].(string))
	}

	c.JSON(http.StatusOK, gin.H{"rooms": rooms})
}

func getClientsInRoom(c *gin.Context) {
	log.Printf("listclients @@@@")
	roomName := c.Query("room")
	if roomName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Room name is required"})
		return
	}

	// Fetch clients associated with the room from MongoDB
	cursor, err := clientsCol.Find(ctx, bson.M{"room": roomName})
	if err != nil {
		log.Printf("Error fetching clients: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve clients"})
		return
	}
	defer cursor.Close(ctx)

	var clientIDs []string
	for cursor.Next(ctx) {
		var client bson.M
		if err := cursor.Decode(&client); err != nil {
			log.Printf("Failed to decode client: %v", err)
			continue
		}
		clientID := client["client_id"].(string)
		clientIDs = append(clientIDs, clientID)
	}

	if err := cursor.Err(); err != nil {
		log.Printf("Cursor error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cursor error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"clients": clientIDs})
}
func getSpecificClientsInRoom(c *gin.Context) {
	log.Printf("listclients @@@@")
	roomName := c.Query("room")
	if roomName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Room name is required"})
		return
	}

	// Fetch clients associated with the room from MongoDB
	cursor, err := clientsCol2.Find(ctx, bson.M{"room": roomName})
	if err != nil {
		log.Printf("Error fetching clients: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve clients"})
		return
	}
	defer cursor.Close(ctx)

	var clientIDs []string
	for cursor.Next(ctx) {
		var client bson.M
		if err := cursor.Decode(&client); err != nil {
			log.Printf("Failed to decode client: %v", err)
			continue
		}
		clientID := client["client_id"].(string)
		clientIDs = append(clientIDs, clientID)
	}

	if err := cursor.Err(); err != nil {
		log.Printf("Cursor error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cursor error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"clients": clientIDs})
}
func deleteUserFromRoom(c *gin.Context) {
	roomName := c.Query("room")
	if roomName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Room name is required"})
		return
	}

	clientID := c.Query("id")
	if clientID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Client ID is required"})
		return
	}
	// roomName := "room1"
	// clientID := "user1"
	hub.mu.Lock()
	defer hub.mu.Unlock()

	room, ok := hub.rooms[roomName]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Room not found"})
		return
	}

	clientExists := false
	var clientToRemove *Client
	for client := range room.clients {
		if client.id == clientID {
			clientExists = true
			clientToRemove = client
			break
		}
	}

	if !clientExists {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found in room"})
		return
	}

	// Unregister the client from the room
	room.unregister <- clientToRemove

	c.JSON(http.StatusOK, gin.H{"message": "User deleted from room"})
}

func saveMessage(c *gin.Context) {
	var msg Message
	if err := c.ShouldBindJSON(&msg); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	log.Printf("message to be saved is: %s", msg)

	msg.Time = time.Now().Format("02-01-2006T03:04:05.000PM07:00")

	// message.RecipientID = "all"
	_, err := mongoClient.Database("chat_app").Collection("messages").InsertOne(context.Background(), msg)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to save message"})
		return
	}
	c.JSON(200, gin.H{"message": "Message saved successfully"})
}
func getMessageByRecipientID(c *gin.Context) {
	roomname := c.Param("roomName")

	// Define filter to match recipient ID
	filter := bson.M{"roomname": roomname}

	// Perform MongoDB query
	cursor, err := mongoClient.Database("chat_app").Collection("recievemessages").Find(context.TODO(), filter)
	if err != nil {
		log.Printf("Failed to find messages: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find messages"})
		return
	}
	defer cursor.Close(context.TODO())

	// Define struct to match the message format
	type Message struct {
		ID          string `bson:"_id,omitempty"`
		Sender      string `bson:"sender"`
		Content     string `bson:"content"`
		RoomName    string `bson:"roomname"`
		Time        string `bson:"time"`
		RecipientID string `bson:"recipientid"`
		Type        string `bson:"type"`
	}

	// Iterate through cursor and decode documents into Message struct
	var messages []Message
	for cursor.Next(context.TODO()) {
		var message Message
		if err := cursor.Decode(&message); err != nil {
			log.Printf("Error decoding message: %v", err)
			continue
		}

		// Log the message
		// log.Printf("Message received: %v", message)

		// Append the message to the messages array
		messages = append(messages, message)
	}

	c.JSON(http.StatusOK, gin.H{"messages": messages})
}


func getMessageByType(c *gin.Context) {
	messageType := c.Param("Type")
	roomName := c.Param("roomName")

	// Define filter to match message type and room name
	filter := bson.M{"type": messageType, "roomname": roomName}
	log.Printf("Message received: %v", messageType)

	// Perform MongoDB query
	cursor, err := mongoClient.Database("chat_app").Collection("recievemessages").Find(context.TODO(), filter)
	if err != nil {
		log.Printf("Failed to find messages: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find messages"})
		return
	}
	defer cursor.Close(context.TODO())

	// Iterate through cursor and decode documents into Message struct
	var messages []Message
	for cursor.Next(context.TODO()) {
		var message Message
		if err := cursor.Decode(&message); err != nil {
			log.Printf("Error decoding message: %v", err)
			continue
		}

		// Append the message to the messages array
		messages = append(messages, message)
	}

	if err := cursor.Err(); err != nil {
		log.Printf("Cursor error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error iterating messages"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"messages": messages})
}
func main() {
	initMongoDB()

	r := gin.Default()
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, Content-Length, X-CSRF-Token, Token, session")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}
		c.Next()
	})

	r.POST("/create", createRoomAndUser) // New API for room and user creation
	r.GET("/ws", serveWs)                // Existing API for sending and receiving data
	r.GET("/rooms", listRooms)
	r.GET("/clients", getClientsInRoom)
	r.DELETE("/deleteuser", deleteUserFromRoom)
	r.GET("/recievemessages/:roomName", getMessageByRecipientID)
	r.GET("/recieveType/:roomName/:Type", getMessageByType)
	r.POST("/recievemessages", saveMessage)
	r.POST("/createaddspecificuser", createRoomAndAddUsers)
	r.GET("/getspecificuserroom", listSpecificRooms)
	r.GET("/getspecificuerclients", getSpecificClientsInRoom)
	log.Println("WebSocket server started on :8000")
	if err := r.Run(":8000"); err != nil {
		log.Fatalf("ListenAndServe: %v", err)
	}
}
