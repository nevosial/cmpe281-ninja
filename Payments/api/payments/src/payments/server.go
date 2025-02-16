package main

import (
	"fmt"
	"log"
	"net/http"
	"encoding/json"
	"github.com/codegangsta/negroni"
	"github.com/gorilla/mux"
	"github.com/unrolled/render"
	"gopkg.in/mgo.v2"
    "gopkg.in/mgo.v2/bson"
    "time"
    "bytes"
    "strings"
    "net"
    "io/ioutil"
    "strconv"
)

/* 

Variable declaration and initialization

*/

// MongoDB Configuration

//Local DB payment configuration 
/*var mongodb_server = "localhost:27017"
var mongodb_database = "payments"
var mongodb_collection = "payment"*/

//Local DB payment log configuration 
var mongodb_log_server = "localhost:27017"
var mongodb_log_database = "logSystem"
var mongodb_log_collection = "paymentactivitylogs"

//DB payment configuration 

var mongodb_server = "mongodb://54.183.209.156:27017,54.215.133.223:27018,54.183.237.206:27019/ninjacart?replicaSet=mongo-replica-set"
var mongodb_database = "ninjacart"
var mongodb_collection = "payments"

//DB payment log configuration 

//var mongodb_log_server = "localhost:27015"
//var mongodb_log_database = "logSystem"
//var mongodb_log_collection = "paymentactivitylogs"


/*
Function is used to initialize parameters and create a session for logger DB
*/
func init(){

	fmt.Println("Initializing...")

	//Create channel to hold payment object
	Payment_channel=make(chan payment,10)

	//Code to create DB writer workers
	fmt.Println("Starting write workers..")
	for i:=0; i<4; i++{
		fmt.Println("Worker ",i+1,": Started")
		go writerWorker()
	} 
	//Code to create a session to the logging module DB
	sess,err := mgo.Dial(mongodb_log_server)
	if(err!=nil){
		fmt.Println("Unable to connect to logger DB...Proceeding without logging to Logger sub-module")
	}else{
		mw = &MongoWriter{sess}
		log.SetOutput(mw)
	}
}

/*
Log Writer implementation to write logs to Logging module
*/
func (mw *MongoWriter) Write(p []byte) (n int, err error) {
	var log_info = strings.Split(string(p)," ");
	

	client := http.Client{}
	var jsonprep string = `{"_id":"`+strings.TrimSpace(log_info[2])+`"}`
    var jsonStr = []byte(jsonprep)
    url:="http://localhost:7000/getEmail"
    req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonStr))
    req.Header.Set("Content-Type", "application/json")
    resp, err := client.Do(req)

   	body, err := ioutil.ReadAll(resp.Body)
  	

    email:=strings.Split(strings.Split(string(body),",")[0],":")[1]
    email = email[1:len(email)-1]
    amount,_:=strconv.ParseFloat(strings.TrimSpace(log_info[3]), 64)

   c := mw.sess.DB(mongodb_log_database).C(mongodb_log_collection)
    err = c.Insert(bson.M{
        "timestamp": time.Now(),
        "username": email,
        "message" : email+" has paid successfully",
        "amount": amount,//Convert to float
		"requestUrl": strings.TrimSpace(log_info[4]),
		"remoteIp":strings.TrimSpace(log_info[5])})
    if err != nil {
        return
    }
    return len(p), nil
}

/* 
NewServer configures and returns a Server.
*/
func NewServer() *negroni.Negroni {
	formatter := render.New(render.Options{
		IndentJSON: true,
	})
	n := negroni.Classic()
	mx := mux.NewRouter()
	initRoutes(mx, formatter)
	n.UseHandler(mx)
	fmt.Println("Started payment server...")
	return n
}


/*
Function that binds handlers for API routes
*/
func initRoutes(mx *mux.Router, formatter *render.Render) {
	mx.HandleFunc("/payment/{order_id}", paymentHandler(formatter)).Methods("GET")
	mx.HandleFunc("/payment", newPaymentHandler(formatter)).Methods("POST")
	mx.HandleFunc("/ping", ping(formatter)).Methods("GET")
}

/*
Function to provide API healthcheck - ping 
*/
func ping(formatter *render.Render) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		fmt.Println("Ping - Payment API running");
		result := "Payment API - Running"
		formatter.JSON(w, http.StatusOK, result);
	}
}
/*
Function that handles GET request and displays the payment information based on the order number
*/
func paymentHandler(formatter *render.Render) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		// Connects to MongoDB
		session, err := mgo.Dial(mongodb_server)
        if err != nil {
        		fmt.Println("Payments API - Unable to connect to MongoDB during read operation")
                panic(err)
        }
        defer session.Close()
        session.SetMode(mgo.Monotonic, true)
        c := session.DB(mongodb_database).C(mongodb_collection)
        params := mux.Vars(req)
        var order_id string = params["order_id"]
        fmt.Println("\nFetching order#: ",order_id)
        var result bson.M
		err = c.Find(bson.M{"orderId" : order_id}).One(&result)
		if err != nil {
                log.Fatal(mux.Vars(req))
        }
        fmt.Println("\nPayment made is:\n", result)
		formatter.JSON(w, http.StatusOK, result)

	}
}
/*
Function that handles POST request and writes the payment object to Payment Channel
*/
func newPaymentHandler(formatter *render.Render) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		var data payment
		err := json.NewDecoder(req.Body).Decode(&data)
		ip,_,_:=net.SplitHostPort(req.RemoteAddr)
		if err!=nil{
			fmt.Println("Payments API - Unable to obtain request body")
			panic(err)
		}
		log.Println(data.CustomerId,data.Total,req.URL.Path,ip)
		fmt.Println("New payment received")
		fmt.Println("Payment recorded in DB")
		go workerHandler(data,Payment_channel)
		formatter.JSON(w, http.StatusOK, data)
	}
	/*

	Sample payment write:

	URL: http://localhost:3000/payment

	Object:

	{"OrderId" : "1234",	
	"CustomerId" : "Sam", 	
	"Total" : 32.5, 
	"Timestamp" : "20170202"}
	*/

}
/*
Function to write to Payment channel
*/
func workerHandler(data payment,Payment_channel chan payment) {
	// Write payment object to payment channel
	Payment_channel<-data
}
/*
Worker function that write Payment object from the Payment Channel to MongoDB
*/
func writerWorker(){
	// Worker function that gets the value from the payment channel and writes to MongoDB
	for i:=0;;i++{
		payment_value:=<-Payment_channel
		session, err := mgo.Dial(mongodb_server)
        if err != nil {
        		fmt.Println("Payments API - Unable to connect to MongoDB during write operation")
                panic(err)
        }
        defer session.Close()
        session.SetMode(mgo.Monotonic, true)
        c := session.DB(mongodb_database).C(mongodb_collection)
		c.Insert(payment_value)

	}
}

