package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/alertmanager/client"
	"github.com/prometheus/alertmanager/notify"
	"github.com/prometheus/alertmanager/template"
	"github.com/prometheus/alertmanager/types"
	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/common/model"
)

const (
	prometheusListening = ":8081"
)

func promAlertHandler(w http.ResponseWriter, r *http.Request) {
	var msg notify.WebhookMessage

	reqLog := log.WithField("remote_addr", r.RemoteAddr)
	if r.Method != http.MethodPost {
		reqLog.Errorf("Method %s not allowed", r.Method)
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		reqLog.WithError(err).Error("Failed to decode request body")
		http.Error(w, "", http.StatusBadRequest)
		return
	}
	if msg.Data == nil || msg.Alerts == nil {
		reqLog.Errorln("POST message without properly formatted data - refusing to continue")
		return
	}
	reqLog.Debugf("Unmarshalled JSON: %#v", msg.Data)
	reqLog.Debugf("Notification contains %d alert(s)", len(msg.Alerts))
	commonLabels, _ := json.Marshal(msg.CommonLabels)

	var buttons []*genericButton

	for _, alert := range msg.Alerts {
		button := &genericButton{
			HeaderText:  alert.Status,
			ContentText: alert.Annotations["summary"],
			FooterText:  alert.Labels["instance"],
			ButtonText:  "f()",
			OnClickLink: alert.GeneratorURL,
		}
		buttons = append(buttons, button)
	}
	buttons = append(buttons, &genericButton{
		ContentText:      "Snooze",
		ButtonText:       "Snooze 1h",
		CallbackFunction: "prom_silence_1h",
		CallbackInfos: map[string]string{
			"labels":          string(commonLabels),
			"alertMgrAddress": msg.ExternalURL,
		},
	})

	message := &genericMessage{
		HeaderText:       "Prometheus alert",
		FooterText:       fmt.Sprintf("Alert for group %s", msg.Receiver),
		HeaderPictureURL: "https://raw.githubusercontent.com/cncf/artwork/master/prometheus/icon/color/prometheus-icon-color.png",
		Buttons:          buttons,
	}
	hangoutsUser := getHangoutsUsersForAlertGroup(msg.Receiver)
	for user := range hangoutsUser {
		user.sendMessage(message)
	}
}

func startPrometheusListener() {
	log.Infof("Starting prometheus receiver on %s", prometheusListening)

	http.HandleFunc("/alert", promAlertHandler)
	log.Fatal(http.ListenAndServe(prometheusListening, nil))
}

func silenceWithLabels(labels template.KV, username string, alertMgrAddress string) {
	var matchers types.Matchers
	for key, value := range labels {
		match := types.NewMatcher(model.LabelName(key), value)
		matchers = append(matchers, match)
	}
	silence := types.Silence{
		Matchers:  matchers,
		CreatedBy: username,
		Comment:   "botanist snooze",
		EndsAt:    time.Now().Add(time.Minute * 5),
	}
	err := addSilence(silence, alertMgrAddress)
	if err != nil {
		log.Fatalf("Issues when adding silence in alertmanager: %s", err)
	}
}

func addSilence(silence types.Silence, alertMgrAddress string) error {
	apiClient, err := api.NewClient(api.Config{Address: alertMgrAddress})
	if err != nil {
		return err
	}
	silenceAPI := client.NewSilenceAPI(apiClient)
	silenceID, err := silenceAPI.Set(ctx, silence)
	if err != nil {
		return err
	}
	log.Infof("return: %s", silenceID)
	return nil
}
