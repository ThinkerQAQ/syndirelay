package publisher

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestToutiaoUploadBinaryMatchesCapturedSpiceContract(t *testing.T) {
	payload := []byte("a test image body")
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/spice/image" || req.Method != http.MethodPost {
			t.Fatalf("unexpected upload endpoint: %s %s", req.Method, req.URL)
		}
		q := req.URL.Query()
		if q.Get("upload_source") != "20020002" || q.Get("aid") != "1231" || q.Get("device_platform") != "web" || q.Has("need_cover_url") {
			t.Fatalf("unexpected binary upload query: %v", q)
		}
		if !strings.HasPrefix(req.Header.Get("Content-Type"), "multipart/form-data;") {
			t.Fatalf("unexpected body type: %q", req.Header.Get("Content-Type"))
		}
		if err := req.ParseMultipartForm(1024); err != nil {
			t.Fatal(err)
		}
		if len(req.MultipartForm.File["image"]) != 1 || len(req.MultipartForm.Value["imageUrl"]) != 0 {
			t.Fatalf("binary image must be uploaded with file field 'image', fields: %+v", req.MultipartForm)
		}
		part, err := req.MultipartForm.File["image"][0].Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(part)
		part.Close()
		if err != nil || !bytes.Equal(data, payload) {
			t.Fatalf("payload changed, err=%v", err)
		}
		return jsonResponse(req, 200, `{"code":0,"message":"success","data":{"image_uri":"tos-cn-i-6w9my0ksvp/example","image_url":"https://image-tt-private.toutiao.com/img.jpg","image_width":1672,"image_height":941}}`, nil), nil
	})}
	adapterValue, err := NewToutiaoAdapter(client, toutiaoTestSession())
	if err != nil {
		t.Fatal(err)
	}
	target, err := adapterValue.(*toutiaoAdapter).uploadBinary(context.Background(), RehostImage{Source: "/images/photo.jpg", ContentType: "image/jpeg", Payload: payload})
	if err != nil || target != "https://image-tt-private.toutiao.com/img.jpg" {
		t.Fatalf("binary image target=%q err=%v", target, err)
	}
}

func TestToutiaoUploadRemoteURLMatchesCapturedSpiceContract(t *testing.T) {
	source := "https://example.invalid/photo.png"
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/spice/image" || req.Method != http.MethodPost {
			t.Fatalf("unexpected endpoint: %s", req.URL)
		}
		q := req.URL.Query()
		if q.Get("upload_source") != "20020003" || q.Get("need_cover_url") != "1" || q.Get("aid") != "1231" || q.Get("device_platform") != "web" {
			t.Fatalf("unexpected remote upload query: %v", q)
		}
		if err := req.ParseMultipartForm(1024); err != nil {
			t.Fatal(err)
		}
		if req.MultipartForm.Value["imageUrl"][0] != source || len(req.MultipartForm.File) != 0 {
			t.Fatalf("wrong image URL form fields: %+v", req.MultipartForm)
		}
		return jsonResponse(req, 200, `{"code":0,"message":"success","data":{"image_uri":"tos-cn-i-6w9my0ksvp/example","image_url":"https://image-tt-private.toutiao.com/img2.png","cover_url":"https://image-tt-private.toutiao.com/cover.png"}}`, nil), nil
	})}
	adapterValue, _ := NewToutiaoAdapter(client, toutiaoTestSession())
	target, err := adapterValue.(*toutiaoAdapter).uploadByURL(context.Background(), source)
	if err != nil || target != "https://image-tt-private.toutiao.com/img2.png" {
		t.Fatalf("remote target=%q err=%v", target, err)
	}
}

func TestToutiaoImageUploadRejectsBusinessFailuresAndMissingURL(t *testing.T) {
	for name, response := range map[string]string{
		"invalid business code": `{"code":4001,"message":"error","data":{"image_url":"https://example.invalid/should-not-use.jpg"}}`,
		"missing business code": `{"data":{"image_url":"https://example.invalid/should-not-use.jpg"}}`,
		"missing image URL":     `{"code":0,"data":{"image_uri":"tos-cn-i-6w9my0ksvp/example"}}`,
		"non URL":               `{"code":0,"data":{"image_url":"javascript:bad()"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return jsonResponse(req, 200, response, nil), nil
			})}
			adapterValue, _ := NewToutiaoAdapter(client, toutiaoTestSession())
			_, err := adapterValue.(*toutiaoAdapter).uploadByURL(context.Background(), "https://example.invalid/origin.png")
			if err == nil || !IsKind(err, ErrUpload) {
				t.Fatalf("invalid image response succeeded, err=%v", err)
			}
		})
	}
}

func TestToutiaoCapturedEditorHeadersAreForwardedOnlyToWrites(t *testing.T) {
	session := toutiaoTestSession()
	session.RequestHeaders = map[string]string{"x-secsdk-csrf-token": "csrf-test", "tt-anti-token": "anti-test"}
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("x-secsdk-csrf-token") != "csrf-test" || req.Header.Get("tt-anti-token") != "anti-test" {
			t.Fatal("missing captured security header on image upload")
		}
		return jsonResponse(req, 200, `{"code":0,"data":{"image_url":"https://image-tt-private.toutiao.com/upload.jpg"}}`, nil), nil
	})}
	adapterValue, err := NewToutiaoAdapter(client, session)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapterValue.(*toutiaoAdapter).uploadByURL(context.Background(), "https://example.invalid/pic.png")
	if err != nil {
		t.Fatal(err)
	}
	get, err := adapterValue.(*toutiaoAdapter).request(context.Background(), http.MethodGet, "https://mp.toutiao.com/mp/agw/media/get_media_info", nil)
	if err != nil {
		t.Fatal(err)
	}
	if get.Header.Get("tt-anti-token") != "" || get.Header.Get("x-secsdk-csrf-token") != "" {
		t.Fatal("captured editor headers must not be sent to unrelated endpoints")
	}
}

func TestToutiaoUploadedImageIsRecognized(t *testing.T) {
	if !isToutiaoImage("https://image-tt-private.toutiao.com/image.jpg") {
		t.Fatal("creator images must not be uploaded again")
	}
	if isToutiaoImage("https://attack.toutiaoimg.com.evil.test/image.jpg") {
		t.Fatal("non-Toutiao host accepted")
	}
}
