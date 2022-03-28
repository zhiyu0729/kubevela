package application

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	controller "github.com/oam-dev/kubevela/pkg/controller/core.oam.dev"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/oam/discoverymapper"
	"github.com/oam-dev/kubevela/pkg/oam/util"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// MutatingHandler handles application
type MutatingHandler struct {
	Client client.Client

	// Decoder decodes objects
	Decoder *admission.Decoder

	dm                discoverymapper.DiscoveryMapper
	userImpersonation bool
}

var _ admission.Handler = &MutatingHandler{}

// Handle handles admission requests.
func (mh *MutatingHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	obj := &v1beta1.Application{}

	if err := mh.Decoder.Decode(req, obj); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	switch req.Operation {
	case admissionv1.Create:
		if mh.userImpersonation {
			injectUserInfo(obj, req)
		}
	case admissionv1.Update:
		if mh.userImpersonation {
			injectUserInfo(obj, req)
		}
	case admissionv1.Delete:
		if mh.userImpersonation {
			injectUserInfo(obj, req)
		}
	default:
		// nothing to do
	}

	marshalled, err := json.Marshal(obj)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	resp := admission.PatchResponseFromRaw(req.AdmissionRequest.Object.Raw, marshalled)
	if len(resp.Patches) > 0 {
		klog.InfoS("Admit application", "application", klog.KObj(obj), util.JSONMarshal(resp.Patches))
	}
	return resp
}

func injectUserInfo(app *v1beta1.Application, req admission.Request) {
	if app.Annotations == nil {
		app.Annotations = map[string]string{}
	}
	userInfo := req.UserInfo
	if userInfo.Username != "" {
		app.Annotations[oam.AnnotationUserInfoName] = userInfo.Username
	}
	if userInfo.Groups != nil && len(userInfo.Groups) > 0 {
		groups := append([]string{}, userInfo.Groups...)
		sort.Strings(groups)
		app.Annotations[oam.AnnotationUserInfoGroup] = strings.Join(groups, ",")
	}
	if userInfo.Extra != nil && len(userInfo.Extra) > 0 {
		extra, _ := json.Marshal(userInfo.Extra)
		if extra != nil {
			app.Annotations[oam.AnnotationUserInfoExtra] = string(extra)
		}
	}
}

var _ inject.Client = &MutatingHandler{}

// InjectClient injects the client into the ComponentMutatingHandler
func (mh *MutatingHandler) InjectClient(c client.Client) error {
	mh.Client = c
	return nil
}

var _ admission.DecoderInjector = &MutatingHandler{}

// InjectDecoder injects the decoder into the ComponentMutatingHandler
func (mh *MutatingHandler) InjectDecoder(d *admission.Decoder) error {
	mh.Decoder = d
	return nil
}

// RegisterMutatingHandler will register application mutation handler to the webhook
func RegisterMutatingHandler(mgr manager.Manager, args controller.Args) {
	server := mgr.GetWebhookServer()
	if args.UserImpersonation {
		server.Register("/mutating-core-oam-dev-v1beta1-applications", &webhook.Admission{
			Handler: &MutatingHandler{dm: args.DiscoveryMapper, userImpersonation: args.UserImpersonation}},
		)
	}
}
