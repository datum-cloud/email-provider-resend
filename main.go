package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.miloapis.com/email-provider-resend/internal/emailprovider"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

func getenvOrDefault(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func main() {
	token := strings.TrimSpace(os.Getenv("LOOPS_TOKEN"))
	if token == "" {
		fmt.Println("LOOPS_TOKEN is required. Example:")
		fmt.Println("  LOOPS_TOKEN=xxxxx go run ./cmd/loops-demo")
		os.Exit(1)
	}

	email := "joseszychowski@gmail.com"
	firstName := ""
	lastName := ""
	userID := "1qwrqwrqwqeqrr"

	client := emailprovider.NewLoopsEmail(token)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	fmt.Println("== Create contact ==")
	createResp, err := client.CreateContact(ctx, email, firstName, lastName, userID)
	if err != nil {
		if k8serrors.IsConflict(err) {
			err := fmt.Errorf("create conflict (already existss): %w", err)
			fmt.Println(err)
		} else {
			fmt.Printf("Create failed: %v\n", err)
		}
	} else {
		fmt.Printf("Create success=%v, id=%s\n", createResp.Success, createResp.ID)
	}

	mailingLists := []emailprovider.LoopsMailingList{
		{ID: "cmh1115fg060y0iys4s8l817s", Subscribed: true},
		{ID: "cmhtdlxpi0asc0i1l0q89fmzs", Subscribed: false},
	}

	fmt.Println("\n== Update contact ==")
	updateResp, err := client.UpdateContact(ctx, "joseszychowski@gmail.com", firstName+"-Updated", "rey", userID, mailingLists)
	if err != nil {
		if k8serrors.IsConflict(err) {
			fmt.Printf("Update conflict: %v\n", err)
		} else {
			fmt.Printf("Update failed: %v\n", err)
		}
	} else {
		if len(updateResp) > 0 {
			fmt.Printf("Update returned %d contact(s); first id=%s, email=%s , first name=%s, last name=%s\n", len(updateResp), updateResp[0].ID, updateResp[0].Email, updateResp[0].FirstName, updateResp[0].LastName)
		} else {
			fmt.Println("Update returned 0 contacts")
		}
	}

	fmt.Println("\n== Find contact by userID ==")
	findResp, err := client.FindContactByUserID(ctx, "d9f1a94d-62bf-4a5e-8f1e-1314f109d274")
	if err != nil {
		if k8serrors.IsNotFound(err) {
			fmt.Printf("Find: not found for userID=%s: %v\n", userID, err)
		} else {
			fmt.Printf("Find failed: %v\n", err)
		}
	} else {
		if len(findResp) == 0 {
			fmt.Println("Find returned 0 contacts")
		} else {
			first := findResp[0]
			uid := ""

			fmt.Printf("Find returned %d contact(s); first id=%s, email=%s, userId=%s, first name=%s, last name=%s \n", len(findResp), first.ID, first.Email, uid, first.FirstName, first.LastName)
		}
	}

	fmt.Println("\n== Delete contact ==")
	deleteResp, err := client.DeleteContact(ctx, "d9f1a94d-62bf-4a5e-8f1e-1314f109d274")
	if err != nil {
		if k8serrors.IsNotFound(err) {
			fmt.Printf("Delete: not found: %v\n", err)
		} else {
			fmt.Printf("Delete failed: %v\n", err)
		}
	} else {
		fmt.Printf("Delete success=%v, message=%q\n", deleteResp.Success, deleteResp.Message)
	}
}
