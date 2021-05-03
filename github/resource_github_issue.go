package github

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/go-github/v32/github"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"
)

func resourceGithubIssue() *schema.Resource {
	return &schema.Resource{
		Create: resourceGithubIssueCreate,
		Read:   resourceGithubIssueRead,
		Update: resourceGithubIssueUpdate,
		Delete: resourceGithubIssueDelete,
		Importer: &schema.ResourceImporter{
			State: func(d *schema.ResourceData, meta interface{}) ([]*schema.ResourceData, error) {
				parts := strings.Split(d.Id(), "/")
				if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
					return nil, fmt.Errorf("Invalid ID format, must be provided as OWNER/REPOSITORY/NUMBER")
				}
				d.Set("owner", parts[0])
				d.Set("repository", parts[1])
				number, err := strconv.Atoi(parts[2])
				if err != nil {
					return nil, err
				}
				d.Set("number", number)
				d.SetId(fmt.Sprintf("%s/%s/%d", parts[0], parts[1], number))

				return []*schema.ResourceData{d}, nil
			},
		},
		Schema: map[string]*schema.Schema{
			"owner": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"repository": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"lock_reason": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "Controlled by Terraform",
			},
			"title": {
				Type:     schema.TypeString,
				Required: true,
			},
			"body": {
				Type:     schema.TypeString,
				Required: true,
			},
			"state": {
				Type:     schema.TypeString,
				Optional: true,
				ValidateFunc: validation.StringInSlice([]string{
					"open", "closed",
				}, true),
				Default: "open",
			},
			"labels": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"assignees": {
				Type:     schema.TypeList,
				Optional: true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"milestone_number": {
				Type:     schema.TypeInt,
				Optional: true,
			},
			"etag": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"number": {
				Type:     schema.TypeInt,
				Computed: true,
			},
			"issue_id": {
				Type:     schema.TypeInt,
				Computed: true,
			},
		},
	}
}

func resourceGithubIssueCreate(d *schema.ResourceData, meta interface{}) error {

	milestoneNumber := d.Get("milestone_number").(int)

	owner := d.Get("owner").(string)
	repo := d.Get("repository").(string)
	assignees := expandStringList(d.Get("assignees").([]interface{}))
	labels := expandStringList(d.Get("labels").([]interface{}))

	log.Printf("[DEBUG] Creating issue with owner: %s", owner)
	client := meta.(*Owner).v3client
	req := github.IssueRequest{
		Title:     github.String(d.Get("title").(string)),
		Body:      github.String(d.Get("body").(string)),
		Assignees: &assignees,
		Labels:    &labels,
		Milestone: github.Int(milestoneNumber),
	}

	ctx := context.Background()
	issue, _, err := client.Issues.Create(ctx, owner, repo, &req)
	if err != nil {
		return err
	}

	d.Set("state", issue.GetState())
	d.Set("issue_id", issue.GetID())
	d.Set("number", issue.GetNumber())
	d.SetId(issue.GetNodeID())

	return resourceGithubIssueRead(d, meta)
}

func resourceGithubIssueRead(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*Owner).v3client
	nodeID := d.Id()
	issueNumber := d.Get("number").(int)
	ctx := context.WithValue(context.Background(), ctxId, d.Id())
	if !d.IsNewResource() {
		ctx = context.WithValue(ctx, ctxEtag, d.Get("etag").(string))
	}

	owner := d.Get("owner").(string)
	repo := d.Get("repository").(string)

	log.Printf("[DEBUG] Reading issue: %s", nodeID)
	issue, _, err := client.Issues.Get(ctx, owner, repo, issueNumber)
	if err != nil {
		if err, ok := err.(*github.ErrorResponse); ok {
			if err.Response.StatusCode == http.StatusNotFound {
				log.Printf("[WARN] Removing issue %s from state because it no longer exists in GitHub", d.Id())
				d.SetId("")
				return nil
			}
		}
		return err
	}

	d.Set("labels", expandIssueLabels(issue.Labels))
	d.Set("labels", expandIssueLabels(issue.Labels))
	d.Set("assigness", expandIssueUsers(issue.Assignees))
	d.Set("state", issue.GetState())
	d.Set("body", issue.GetBody())
	d.Set("title", issue.GetTitle())
	milestone := issue.GetMilestone()
	if milestone != nil {
		d.Set("milestone_number", milestone.Number)
	}
	d.Set("number", issue.GetNumber())
	d.Set("issue_id", issue.GetID())

	return nil
}

func resourceGithubIssueUpdate(d *schema.ResourceData, meta interface{}) error {
	milestoneNumber := d.Get("milestone_number").(int)
	issueNumber := d.Get("number").(int)
	owner := d.Get("owner").(string)
	repo := d.Get("repository").(string)
	assignees := expandStringList(d.Get("assignees").([]interface{}))
	labels := expandStringList(d.Get("labels").([]interface{}))

	log.Printf("[DEBUG] Creating issue with owner: %s", owner)
	client := meta.(*Owner).v3client
	req := github.IssueRequest{
		Title:     github.String(d.Get("title").(string)),
		Body:      github.String(d.Get("body").(string)),
		State:     github.String(d.Get("state").(string)),
		Assignees: &assignees,
		Labels:    &labels,
		Milestone: github.Int(milestoneNumber),
	}

	ctx := context.Background()
	_, _, err := client.Issues.Edit(ctx, owner, repo, issueNumber, &req)
	if err != nil {
		return err
	}

	return resourceGithubIssueRead(d, meta)
}

func resourceGithubIssueDelete(d *schema.ResourceData, meta interface{}) error {
	client := meta.(*Owner).v3client
	ctx := context.WithValue(context.Background(), ctxId, d.Id())

	owner := d.Get("owner").(string)
	repo := d.Get("repository").(string)

	log.Printf("[DEBUG] Deleting project Card: %s", d.Id())
	issueNumber := d.Get("number").(int)

	options := github.LockIssueOptions{
		LockReason: d.Get("lock_reason").(string),
	}

	_, err := client.Issues.Lock(ctx, owner, repo, issueNumber, &options)
	if err != nil {
		return err
	}

	return nil
}

func expandIssueUsers(users []*github.User) []string {
	usernames := make([]string, len(users))
	for i, user := range users {
		usernames[i] = github.Stringify(user.Login)
	}
	return usernames
}

func expandIssueLabels(labels []*github.Label) []string {
	labelsNames := make([]string, len(labels))
	for i, user := range labels {
		labelsNames[i] = github.Stringify(user.Name)
	}
	return labelsNames
}
