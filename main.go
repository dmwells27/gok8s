package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/IBM/sarama"
	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type ConnectionConfigManager interface {
	GetConnectionConfig() (*rest.Config, error)
}

type InternalConnectionConfigManager struct{}

func (cm InternalConnectionConfigManager) GetConnectionConfig() (*rest.Config, error) {

	return rest.InClusterConfig()
}

type ExternalConnectionConfigManager struct{}

func (cm ExternalConnectionConfigManager) GetConnectionConfig() (*rest.Config, error) {

	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

func sendMessage() {
	config := sarama.NewConfig()
	producer, err := sarama.NewAsyncProducer([]string{"my-cluster-kafka-brokers.kafka.svc.cluster.local:9092"}, config)
	if err != nil {
		log.Fatalln("Failed to start Sarama producer:", err)
	}
	defer producer.AsyncClose()
	producer.Input() <- &sarama.ProducerMessage{
		Topic: "access_log",
		Key:   sarama.StringEncoder("1"),
		Value: sarama.StringEncoder("some value"),
	}
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/", HomeHandler)
	r.HandleFunc("/k8s", getKubernetesInfo)
	http.Handle("/", r)

	srv := &http.Server{
		Handler:      r,
		Addr:         "0.0.0.0:8000",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}
	log.Println("Starting...")
	err := srv.ListenAndServe()
	if err != nil {
		return
	}
}

type MyRequest struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type MyResponse struct {
	Count  int    `json:"count"`
	Status string `json:"status"`
}

func HomeHandler(writer http.ResponseWriter, request *http.Request) {
	log.Println("got request")
	sendMessage()
	//body, err := io.ReadAll(request.Body)
	//if err != nil {
	//	return
	//}
	//myRequest := MyRequest{}
	//err = json.Unmarshal(body, &myRequest)
	//if err != nil {
	//	return
	//}
	//log.Println(myRequest.ID)
	//_, err = writer.Write([]byte("some response"))
	//if err != nil {
	//	return
	//}

	conn, err := sql.Open("postgres", "postgres://root@my-cockroachdb-public.cockroachdb.svc.cluster.local:26257/defaultdb?sslmode=disable")
	if err != nil {
		panic(err.Error())
	}
	rows, err := conn.Query("select id, name from customer")
	if err != nil {
		panic(err.Error())
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id   int64
			name string
		)
		if err := rows.Scan(&id, &name); err != nil {
			log.Fatal(err)
		}
		log.Printf("id %d name is %s\n", id, name)
	}
}

func getConnection() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		var kubeconfig *string
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
		} else {
			kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
		}
		//flag.Parse()

		// use the current context in kubeconfig
		config2, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			panic(err.Error())
		}

		config = config2
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return clientset
}

func getKubernetesInfo(writer http.ResponseWriter, request *http.Request) {
	var im ConnectionConfigManager = ExternalConnectionConfigManager{}
	config, err := im.GetConnectionConfig()
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	// get pods in all the namespaces by omitting namespace
	// Or specify namespace to get pods in particular namespace
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("There are %d pods in the cluster\n", len(pods.Items))

	var response *MyResponse = &MyResponse{}
	response.Count = len(pods.Items)

	// Examples for error handling:
	// - Use helper functions e.g. errors.IsNotFound()
	// - And/or cast to StatusError and use its properties like e.g. ErrStatus.Message
	_, err = clientset.CoreV1().Pods("default").Get(context.TODO(), "example-xxxxx", metav1.GetOptions{})
	if errors.IsNotFound(err) {
		fmt.Printf("Pod example-xxxxx not found in default namespace\n")
		response.Status = "Pod example-xxxxx not found in default namespace"
	} else if statusError, isStatus := err.(*errors.StatusError); isStatus {
		fmt.Printf("Error getting pod %v\n", statusError.ErrStatus.Message)
	} else if err != nil {
		panic(err.Error())
	} else {
		fmt.Printf("Found example-xxxxx pod in default namespace\n")
	}

	marshal, err := json.Marshal(response)
	if err != nil {
		return
	}
	_, err = writer.Write(marshal)
	if err != nil {
		return
	}

}
