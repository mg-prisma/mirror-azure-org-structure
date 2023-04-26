package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

var TYPE_MANAGEMENT_GROUPS = "Microsoft.Management/managementGroups"
var TYPE_SUBSCRIPTIONS = "Microsoft.Management/managementGroups/subscriptions"

type Results struct {
	Value []ManagementGroupInfo `json:"value"`
}

type ManagementGroupInfo struct {
	Id                    string             `json:"id"`
	Name                  string             `json:"name"`
	Properties            SubfieldProperties `json:"properties"`
	Type                  string             `json:"type"`
	ChildrenGroups        []int
	ChildrenSubscriptions []int
}

type SubfieldProperties struct {
	DisplayName string         `json:"displayName"`
	Parent      SubfieldParent `json:"parent"`
}

type SubfieldParent struct {
	Id string `json:"id"`
}

func main() {
	azureSub := os.Getenv("AZURE_SUBSCRIPTION_ID")
	azureTenant := os.Getenv("AZURE_TENANT_ID")
	azureAccessToken := os.Getenv("AZURE_ACCESS_TOKEN")
	if azureSub == "" || azureTenant == "" || azureAccessToken == "" {
		log.Fatal("ERROR: must specify environment variables: AZURE_SUBSCRIPTION_ID, AZURE_TENANT_ID, and AZURE_ACCESS_TOKEN. Use command `az account get-access-token` to get this info.")
	}
	client := &http.Client{}
	req, err := http.NewRequest("GET",
		"https://management.azure.com/providers/Microsoft.Management/managementGroups/"+azureTenant+"/descendants?api-version=2021-04-01",
		nil)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Add("Authorization", "Bearer "+azureAccessToken)
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	r := Results{}
	json.Unmarshal(respBytes, &r)
	resp.Body.Close()

	// groupsAndSubs is our static list of all management groups and subscriptions
	groupsAndSubs := r.Value
	// idToIndexMap is a lookup table that maps the mgmt group ID or subscription ID
	// to its index in the "groupsAndSubs" list above
	idToIndexMap := make(map[string]int)
	idToChildGroupsMap := make(map[string][]int)
	idToChildSubscriptionsMap := make(map[string][]int)

	rootGroup := ManagementGroupInfo{Id: "/providers/Microsoft.Management/managementGroups/" + azureTenant,
		Type: TYPE_MANAGEMENT_GROUPS, Properties: SubfieldProperties{DisplayName: "TENANT_ROOT"}}

	groupsAndSubs = append(groupsAndSubs, rootGroup)
	idToIndexMap[rootGroup.Id] = len(groupsAndSubs) - 1

	log.Printf("There are %d groups and subscriptions", len(groupsAndSubs))

	for i, v := range groupsAndSubs {
		// populate the id to index map
		idToIndexMap[v.Id] = i
		pId := v.Properties.Parent.Id
		if pId != "" {
			// if the current item has a parent,
			// append the current item to the parent's array of children
			if v.Type == TYPE_MANAGEMENT_GROUPS {
				idToChildGroupsMap[pId] = append(idToChildGroupsMap[pId], i)
			} else if v.Type == TYPE_SUBSCRIPTIONS {
				idToChildSubscriptionsMap[pId] = append(idToChildSubscriptionsMap[pId], i)
			}
		}
	}
	//log.Printf("idToChildGroupsMap: %+v", idToChildGroupsMap)

	for i, v := range groupsAndSubs {
		//log.Printf("v.ID: %s", v.Id)
		//log.Printf("idToChildGroupsMap[v.Id]: %+v", idToChildGroupsMap[v.Id])
		groupsAndSubs[i].ChildrenGroups = idToChildGroupsMap[v.Id]
		groupsAndSubs[i].ChildrenSubscriptions = idToChildSubscriptionsMap[v.Id]

		if v.Type == TYPE_MANAGEMENT_GROUPS {
			s := fmt.Sprintf(`resource "prismacloud_account_group" "%s" {
	name = "%s"
	description = "Made by Terraform"
	account_ids = [%s]
	child_group_ids = [%s]
}
`,
				getDisplayName(v)+"---"+formatGroupId(v.Id),
				getDisplayName(v)+"---"+formatGroupId(v.Id),
				formatSubscriptionIds(groupsAndSubs[i].ChildrenSubscriptions, groupsAndSubs),
				formatGroupIdsList(groupsAndSubs[i].ChildrenGroups, groupsAndSubs))
			fmt.Print(s)
		}

	}

	//log.Printf("groupsAndSubs: %+v", groupsAndSubs) DEBUG
}

func formatGroupId(s string) string {
	s = strings.Replace(s, "/providers/Microsoft.Management/managementGroups/", "", -1)
	s = strings.Replace(s, "/subscriptions/", "", -1)
	s = strings.Replace(s, "/", "__", -1)
	return s
}

func formatSubscriptionIds(list []int, groupsAndSubs []ManagementGroupInfo) string {
	s := ""
	for _, v := range list {
		//s += "\"" + getDisplayName(groupsAndSubs[v]) + " - " + formatGroupId(groupsAndSubs[v].Id) + "\" , "
		s += "\"" + formatGroupId(groupsAndSubs[v].Id) + "\" , "
	}

	if len(s) > 3 {
		s = s[0 : len(s)-3] // chop off the last comma
	}

	return s
}

func formatGroupIdsList(list []int, groupsAndSubs []ManagementGroupInfo) string {
	s := ""
	for _, v := range list {
		s += "\"" + getDisplayName(groupsAndSubs[v]) + "---" + formatGroupId(groupsAndSubs[v].Id) + "\" , "
	}

	if len(s) > 3 {
		s = s[0 : len(s)-3] // chop off the last comma
	}

	return s
}

func getDisplayName(m ManagementGroupInfo) string {
	s := strings.Replace(m.Properties.DisplayName, " ", "_", -1)
	if strings.HasPrefix(strings.ToLower(s), "az-ps-") && len(s) > 6 {
		return s[6:]
	}
	return s
}
