package common

import (
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"
	"github.com/mitchellh/multistep"
	"github.com/mitchellh/packer/packer"
)

type StepCreateTags struct {
	Tags map[string]string
}

func (s *StepCreateTags) Run(state multistep.StateBag) multistep.StepAction {
	ec2conn := state.Get("ec2").(*ec2.EC2)
	ui := state.Get("ui").(packer.Ui)
	amis := state.Get("amis").(map[string]string)

	if len(s.Tags) > 0 {
		for region, ami := range amis {
			ui.Say(fmt.Sprintf("Adding tags to AMI (%s)...", ami))

			var ec2Tags []ec2.Tag
			for key, value := range s.Tags {
				ui.Message(fmt.Sprintf("Adding tag: \"%s\": \"%s\"", key, value))
				ec2Tags = append(ec2Tags, ec2.Tag{key, value})
			}

			customClient := &ResilientTransport{
				Deadline: func() time.Time {
					return time.Now().Add(30 * time.Second)
				},
				DialTimeout: 10 * time.Second,
				MaxTries:    100,
				ShouldRetry: func(req *http.Request, res *http.Response, err error) bool {
					ui.Say(fmt.Sprintf("Checking error (%s) to see if should retry.", err))

					retry := false

					// Retry if there's a temporary network error.
					if neterr, ok := err.(net.Error); ok {
						if neterr.Temporary() {
							retry = true
						}
					}

					// Retry if we get a 5xx series error.
					if res != nil {
						if res.StatusCode >= 500 && res.StatusCode < 600 {
							retry = true
						}
					}

					return retry
				},
				Wait: aws.ExpBackoff,
			}

			regionconn := ec2.NewWithClient(ec2conn.Auth, aws.Regions[region], customClient)
			_, err := regionconn.CreateTags([]string{ami}, ec2Tags)
			if err != nil {
				err := fmt.Errorf("Error adding tags to AMI (%s): %s", ami, err)
				state.Put("error", err)
				ui.Error(err.Error())
				return multistep.ActionHalt
			}
		}
	}

	return multistep.ActionContinue
}

func (s *StepCreateTags) Cleanup(state multistep.StateBag) {
	// No cleanup...
}
