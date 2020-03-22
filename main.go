package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

var API_TOKEN string
const SERVICE_URL = "https://slack.com/api/"
var MAIN_CHANNEL_ID string
var MAIN_CHANNEL_NAME string
var CURRENT_THREAD_ID string

type SlackResponse struct {
  Ok bool
  Channels []ConversationList
  Members []string
  Channel map[string]string
  Type string
  Challenge string
  Event SlackEvent
}

type ConversationList struct {
  Id, Name string
  Is_channel, Is_group, Is_im, Is_member, Is_mpim, Is_private bool
}

type SlackEvent struct {
  Type string
  Text string
  User string
}

func StringMapToPostBody(m map[string]string) string {
  if m == nil {
    return ""
  }
  b := new(bytes.Buffer)
	fmt.Fprintf(b, "{")
	for key, value := range m {
		fmt.Fprintf(b, "\"%s\":\"%s\",", key, value)
	}
	if len(m) > 0 {
		b.Truncate(b.Len() - 1)
	}
	fmt.Fprintf(b, "}")
	return b.String()
}

func StringMapToGetBody(m map[string]string, trail bool) string {
	b := new(bytes.Buffer)
	fmt.Fprintf(b, "?")
	for key, value := range m {
		fmt.Fprintf(b, "%s=%s&", key, value)
	}
	if len(m) > 0 && !trail {
		b.Truncate(b.Len() - 2)
	}
	return b.String()
}

func CaptureResponseBody(r io.ReadCloser) string {
	builder := strings.Builder{}

	for {
		bytes := make([]byte, 256)
		length, err := r.Read(bytes)
		builder.Write(bytes[:length])
		if err != nil {
			break
		}
	}

	r.Close()
	return builder.String()
}

func HandleResponse(res *http.Response, err error, logBody bool) string {
	if err != nil {
		log.Println("Error in HandleResponse:")
		log.Println(err)
    return ""
	} else {
		log.Printf("Status: %s", res.Status)
    body := CaptureResponseBody(res.Body)
    if logBody {
		  log.Printf(body)
    }
    return body
	}
}

func DoRequest(url string, req *http.Request) (res *http.Response, err error) {
	req.Header.Add("charset", "utf-8")
	client := http.Client{}

	log.Println("Pre-request")
	defer log.Println("Post-request")
	return client.Do(req)
}

func PerformGet(url string, headers map[string]string, body map[string]string, includeAuth bool) (res *http.Response, err error) {
	if includeAuth {
		url = fmt.Sprintf("%s%s%stoken=%s", SERVICE_URL, url, StringMapToGetBody(body, true), API_TOKEN)
	} else {
		url = fmt.Sprint(SERVICE_URL, url, StringMapToGetBody(body, false))
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	for key, value := range headers {
		req.Header.Add(key, value)
	}

	log.Printf("Performing GET for URL: %s\n", url)
	return DoRequest(url, req)
}

func PerformPost(url string, headers map[string]string, body map[string]string, includeAuth bool) (res *http.Response, err error) {
	url = fmt.Sprint(SERVICE_URL, url)
	req, err := http.NewRequest("POST", url, strings.NewReader(StringMapToPostBody(body)))
	if err != nil {
		return nil, err
	}

	if includeAuth {
		authHeader := fmt.Sprintf("Bearer %s", API_TOKEN)
		req.Header.Add("Authorization", authHeader)
	}
	req.Header.Add("Content-Type", "application/json")

	for key, value := range headers {
		req.Header.Add(key, value)
	}

  log.Printf("Performing POST for URL: %s with Body:\n", url)
  log.Println(StringMapToPostBody(body))
	return DoRequest(url, req)
}

func SendMessage(message, channelId, thread string) {
	params := make(map[string]string)
	params["text"] = message
	if thread == "" {
		params["thread_ts"] = thread
	}
	params["channel"] = channelId

	res, err := PerformPost("chat.postMessage", nil, params, true)

	HandleResponse(res, err, true)
}

func TestSlack(error bool, message string) {
	var params string
	if error {
		fmt.Printf("Error test for %s", message)
		params = fmt.Sprintf("error=%s", message)
	} else {
		fmt.Printf("Testing %s", message)
		params = fmt.Sprintf("test_message=%s", message)
	}
	url := fmt.Sprintf("api.test?%s", params)
	res, err := PerformPost(url, nil, nil, false)

	HandleResponse(res, err, true)
}

func GetChannels(logAnswer bool) (channels map[string]ConversationList) {
	url := "conversations.list"
	res, err := PerformGet(url, nil, nil, true)
  body := HandleResponse(res, err, false)

  var resp SlackResponse
  err = json.Unmarshal([]byte(body), &resp)

	if err != nil || !resp.Ok {
		log.Println("Error in GetChannels:")
		log.Println(err)
    log.Printf("resp.Ok: %t\n", resp.Ok)
    return nil
	}

  channels = make(map[string]ConversationList)
  for _, item := range resp.Channels {
    channels[item.Name] = item
  }
  if logAnswer {
    log.Println(channels)
  }

  if MAIN_CHANNEL_ID == "" {
    MAIN_CHANNEL_ID = channels[MAIN_CHANNEL_NAME].Id
    log.Printf("Main Channel Name: %s, Id: %s\n", MAIN_CHANNEL_NAME, MAIN_CHANNEL_ID)
  }
  return channels
}

func GetUsers(channelId string, logAnswer bool) (users []string) {
  url := "conversations.members"
  params := make(map[string]string)
  params["channel"] = channelId
  res, err := PerformGet(url, nil, params, true)
  body := HandleResponse(res, err, false)

  var resp SlackResponse
  err = json.Unmarshal([]byte(body), &resp)

  if err != nil || !resp.Ok {
    log.Println("Error in GetUsers:")
    log.Println(err)
    log.Printf("resp.Ok: %t\n", resp.Ok)
    return nil
  }

  if logAnswer {
    log.Println(resp.Members)
  }

  return resp.Members
}

func MessageUser(userId, message string) {
  url := "conversations.open"
  params := make(map[string]string)
  params["users"] = userId
  res, err := PerformPost(url, nil, params, true)
  body := HandleResponse(res, err, false)

  var resp SlackResponse 
  err = json.Unmarshal([]byte(body), &resp)
  if err != nil || !resp.Ok {
    log.Println("Error in MessageUser:")
    log.Println(err)
    log.Printf("resp.Ok: %t\n", resp.Ok)
    return 
  }
  newChannelId := resp.Channel["id"]

  SendMessage(message, newChannelId, "")
}


func TestSuccess(w http.ResponseWriter, r *http.Request) {
	TestSlack(false, r.URL.Path)
	w.Write([]byte("Tested Success"))
}

func TestError(w http.ResponseWriter, r *http.Request) {
	TestSlack(true, r.URL.Path)
	w.Write([]byte("Tested Error"))
}

func RunGetChannels(w http.ResponseWriter, r *http.Request) {
	GetChannels(true)
	w.Write([]byte("Channels Retrieved"))
}

func RunGetUsers(w http.ResponseWriter, r *http.Request) {
  if MAIN_CHANNEL_ID == "" {
    GetChannels(true)
  }

  GetUsers(MAIN_CHANNEL_ID, true)
  w.Write([]byte("Users Retrieved"))
}

func RunMessageUser(w http.ResponseWriter, r *http.Request) {
  myId := "UNCHAPM3R"
  MessageUser(myId, "hello")
  w.Write([]byte("Message Sent to User"))
}

func HandleCallback(w http.ResponseWriter, r *http.Request) {
  body := CaptureResponseBody(r.Body)
  var resp SlackResponse
  json.Unmarshal([]byte(body), &resp)
  if resp.Type == "url_verification" {
    w.Write([]byte(resp.Challenge))
    log.Println("Slack API Callback Url Verified")
    return
  } else if resp.Type == "message" {
    if CURRENT_THREAD_ID == "" {
      MessageUser(resp.Event.User, "No instance currently open")
      return
    }
  } 
}

func main() {
  var port string
  if os.Getenv("ENVIRONMENT") == "development" {
  	err := godotenv.Load()
  	if err != nil {
  		log.Fatal(err)
  	}
	  port = os.Getenv("PORT")
  } else {
    port = fmt.Sprintf(":%s", os.Getenv("PORT"))
  }
	API_TOKEN = os.Getenv("API_TOKEN")
  MAIN_CHANNEL_NAME = os.Getenv("MAIN_CHANNEL_NAME")
  if port == "" || port == ":" || API_TOKEN == "" || MAIN_CHANNEL_NAME == "" {
		log.Fatal("PORT, MAIN_CHANNEL_NAME, and API_TOKEN must be set")
	}

  log.Printf("Server starting on Port: %s...\n", port)

	router := mux.NewRouter()

	router.HandleFunc("/", HandleCallback)
	router.HandleFunc("/test", TestSuccess)
	router.HandleFunc("/testError", TestError)
	router.HandleFunc("/getConvos", RunGetChannels)
  router.HandleFunc("/getUsers", RunGetUsers)
  router.HandleFunc("/message", RunMessageUser)
	log.Fatal(http.ListenAndServe(port, router))
}
