package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"io/ioutil"
	"log"
	"net/http"

	"context"

	kserveapi "github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	servingv1beta1 "github.com/kserve/kserve/pkg/client/clientset/versioned/typed/serving/v1beta1"
	kserveconstants "github.com/kserve/kserve/pkg/constants"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

//go:embed static/*
var static embed.FS
var kserve_client *servingv1beta1.ServingV1beta1Client
var namespace string = "default"

type Items struct {
	Items []PredictorStruct
}
type PredictorStruct struct {
	Name            string `json:"name"`
	ModelName       string `json:"modelName"`
	ProtocolVersion string `json:"protocolVersion"`
	StorageUri      string `json:"storageUri"`
}

type formRequest struct {
	Isvctype string
	Url      string
	Isvcname string
}

func list_isvc(client *servingv1beta1.ServingV1beta1Client, ctx context.Context, namespace string) ([]byte, error) {
	isvc_list, err := client.InferenceServices(namespace).List(ctx, metav1.ListOptions{})

	isvc_list_new := make([]PredictorStruct, len(isvc_list.Items))

	for i := 0; i < len(isvc_list.Items); i++ {
		isvc_list_new[i] = PredictorStruct{
			ModelName:       isvc_list.Items[i].Spec.Predictor.Model.ModelFormat.Name,
			ProtocolVersion: string(*isvc_list.Items[i].Spec.Predictor.Model.ProtocolVersion),
			StorageUri:      *isvc_list.Items[i].Spec.Predictor.Model.StorageURI,
			Name:            isvc_list.Items[i].Name,
		}
	}

	items := Items{
		Items: isvc_list_new,
	}
	marshalled_isvclist, err := json.Marshal(items)

	if err != nil {
		return nil, err
	}
	return marshalled_isvclist, nil
}

func delete_isvc(client *servingv1beta1.ServingV1beta1Client, ctx context.Context, namespace string, name string) (string, error) {
	err := client.InferenceServices(namespace).Delete(ctx, name, metav1.DeleteOptions{})

	if err != nil {
		return "{\"message\":\"Error deleting resource\"}", err
	}
	return "{\"message\":\"Successfully deleted resource\"}", nil
}

func create_isvc(ctx context.Context, isvcModel string, name string, uri string, client *servingv1beta1.ServingV1beta1Client, namespace string) (string, error) {
	var svc kserveapi.InferenceService
	switch isvcModel {
	case "onnx":
		protocol := kserveconstants.ProtocolV2

		svc = kserveapi.InferenceService{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "serving.kserve.io/v1beta1",
				Kind:       "InferenceService",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Spec: kserveapi.InferenceServiceSpec{
				Predictor: kserveapi.PredictorSpec{
					ONNX: &kserveapi.ONNXRuntimeSpec{
						PredictorExtensionSpec: kserveapi.PredictorExtensionSpec{
							ProtocolVersion: &protocol,
							StorageURI:      &uri,
							Container: v1.Container{
								Args: []string{
									"--strict-model-config=false",
								},
							},
						},
					},
				},
			},
		}

	case "xgboost":
	case "sklearn":

		protocol := kserveconstants.ProtocolV2

		svc = kserveapi.InferenceService{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "serving.kserve.io/v1beta1",
				Kind:       "InferenceService",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Spec: kserveapi.InferenceServiceSpec{
				Predictor: kserveapi.PredictorSpec{
					SKLearn: &kserveapi.SKLearnSpec{
						PredictorExtensionSpec: kserveapi.PredictorExtensionSpec{
							ProtocolVersion: &protocol,
							StorageURI:      &uri,
						},
					},
				},
			},
		}

	case "tensorflow":
		protocol := kserveconstants.ProtocolV1

		svc = kserveapi.InferenceService{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "serving.kserve.io/v1beta1",
				Kind:       "InferenceService",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},
			Spec: kserveapi.InferenceServiceSpec{
				Predictor: kserveapi.PredictorSpec{
					Tensorflow: &kserveapi.TFServingSpec{
						PredictorExtensionSpec: kserveapi.PredictorExtensionSpec{
							ProtocolVersion: &protocol,
							StorageURI:      &uri,
						},
					},
				},
			},
		}
	}

	_, err := client.InferenceServices(namespace).Create(ctx, &svc, metav1.CreateOptions{})
	if err != nil {
		return "{\"message\":\"Error creating resource\"}", err
	}

	return "{\"message\":\"Successfully submitted\"}", err
}

func main() {
	kubeconfig := "config/config_af_new"
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	kserve_client, _ = servingv1beta1.NewForConfig(config)

	content, _ := fs.Sub(static, "static")
	mutex := http.NewServeMux()
	mutex.Handle("/", http.FileServer(http.FS(content)))
	mutex.HandleFunc("/list_isvc", list_isvc_handler)
	mutex.HandleFunc("/create_isvc", create_isvc_handler)
	mutex.HandleFunc("/delete_isvc", delete_isvc_handler)
	err = http.ListenAndServe(":3000", mutex)
	if err != nil {
		log.Fatal(err)
	}
}

func list_isvc_handler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	svc_list, _ := list_isvc(kserve_client, ctx, namespace)
	w.Write(svc_list)
}

func create_isvc_handler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	bodyBytes, err := ioutil.ReadAll(r.Body)
	form := formRequest{}
	json.Unmarshal(bodyBytes, &form)

	model := form.Isvctype
	storageUri := form.Url
	name := form.Isvcname
	out, err := create_isvc(ctx, model, name, storageUri, kserve_client, namespace)
	if err != nil {
		log.Println(err)
	}
	w.Write([]byte(out))
}

func delete_isvc_handler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	bodyBytes, _ := ioutil.ReadAll(r.Body)
	form := formRequest{}
	json.Unmarshal(bodyBytes, &form)

	name := form.Isvcname
	out, err := delete_isvc(kserve_client, ctx, namespace, name)

	if err != nil {
		log.Println(err)
	}

	w.Write([]byte(out))
}