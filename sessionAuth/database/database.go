package database

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)
var Client *mongo.Client
var UserCollection *mongo.Collection
func ConnectDB() error{
	ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second)
	defer cancel()
	clientOptions:=options.Client().ApplyURI("mongodb://localhost:27017")
	client,err:=mongo.Connect(ctx,clientOptions)
	if err!=nil{
		return err
	}
	err=client.Ping(ctx,nil)
	if err!=nil{
		return err
	}
	UserCollection=client.Database("my_db").Collection("users")
	return nil
}