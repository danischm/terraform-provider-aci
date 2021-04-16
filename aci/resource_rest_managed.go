package aci

import (
	"log"

	"github.com/ciscoecosystem/aci-go-client/client"
	"github.com/ciscoecosystem/aci-go-client/container"
	"github.com/ciscoecosystem/aci-go-client/models"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

const Retries = 2

func resourceAciRestManaged() *schema.Resource {
	return &schema.Resource{
		Create: resourceAciRestManagedCreate,
		Update: resourceAciRestManagedUpdate,
		Read:   resourceAciRestManagedRead,
		Delete: resourceAciRestManagedDelete,

		SchemaVersion: 1,

		Schema: map[string]*schema.Schema{
			"dn": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"class_name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"content": &schema.Schema{
				Type:     schema.TypeMap,
				Optional: true,
				Computed: true,
			},
			"state_ignore_attributes": &schema.Schema{
				Type:     schema.TypeSet,
				Elem:     &schema.Schema{Type: schema.TypeString},
				Optional: true,
			},
		},
	}
}

func getPath(dn string) string {
	return "/api/mo/" + dn + ".json"
}

func getAciRestManaged(d *schema.ResourceData, c *container.Container) error {
	className := d.Get("class_name").(string)
	dn := d.Get("dn").(string)
	d.SetId(dn)

	ignoreAttr := d.Get("state_ignore_attributes")
	ignoreAttrList := toStringList(ignoreAttr.(*schema.Set).List())

	content := d.Get("content")
	contentStrMap := toStrMap(content.(map[string]interface{}))
	newContent := make(map[string]interface{})

	for key, value := range contentStrMap {
		ignore_found := false
		for _, ignoreAttr := range ignoreAttrList {
			if ignoreAttr == key {
				ignore_found = true
				break
			}
		}
		if ignore_found {
			newContent[key] = value
		} else {
			newContent[key] = models.StripQuotes(models.StripSquareBrackets(c.Search("imdata", className, "attributes", key).String()))
		}
	}
	d.Set("content", newContent)
	return nil
}

func resourceAciRestManagedCreate(d *schema.ResourceData, m interface{}) error {
	for attempts := 0; ; attempts++ {
		cont, err := RestPost(d, m)
		if err != nil {
			if attempts >= Retries {
				return err
			} else {
				log.Printf("[ERROR] Failed to create object: %s, retries: %v", err, attempts)
				continue
			}
		}

		err = getAciRestManaged(d, cont)
		if err != nil {
			if attempts >= Retries {
				return err
			} else {
				log.Printf("[ERROR] Failed to decode response after creating object: %s, retries: %v", err, attempts)
				continue
			}
		}
		return nil
	}
}

func resourceAciRestManagedUpdate(d *schema.ResourceData, m interface{}) error {
	for attempts := 0; ; attempts++ {
		cont, err := RestPost(d, m)
		if err != nil {
			if attempts >= Retries {
				return err
			} else {
				log.Printf("[ERROR] Failed to update object: %s, retries: %v", err, attempts)
				continue
			}
		}

		err = getAciRestManaged(d, cont)
		if err != nil {
			if attempts >= Retries {
				return err
			} else {
				log.Printf("[ERROR] Failed to decode response after updating object: %s, retries: %v", err, attempts)
				continue
			}
		}
		return nil
	}
}

func resourceAciRestManagedRead(d *schema.ResourceData, m interface{}) error {
	log.Printf("[DEBUG] %s: Beginning Read", d.Id())

	for attempts := 0; ; attempts++ {
		cont, err := RestGet(d, m)
		if err != nil {
			if attempts >= Retries {
				return err
			} else {
				log.Printf("[ERROR] Failed to read object: %s, retries: %v", err, attempts)
				continue
			}
		}

		// Check if we received an empty response without errors -> object has been deleted
		if cont == nil && err == nil {
			d.SetId("")
			return nil
		}

		err = getAciRestManaged(d, cont)
		if err != nil {
			if attempts >= Retries {
				return err
			} else {
				log.Printf("[ERROR] Failed to decode response after reading object: %s, retries: %v", err, attempts)
				continue
			}
		}

		if err == nil {
			break
		}
	}

	log.Printf("[DEBUG] %s: Read finished successfully", d.Id())

	return nil
}

func resourceAciRestManagedDelete(d *schema.ResourceData, m interface{}) error {
	log.Printf("[DEBUG] %s: Beginning Destroy", d.Id())

	aciClient := m.(*client.Client)
	dn := d.Id()
	className := d.Get("class_name").(string)
	var err error
	for attempts := 0; ; attempts++ {
		err = aciClient.DeleteByDn(dn, className)
		if err != nil && attempts >= Retries {
			if attempts >= Retries {
				return err
			} else {
				log.Printf("[ERROR] Failed to delete object: %s, retries: %v", err, attempts)
				continue
			}
		}
		break
	}

	log.Printf("[DEBUG] %s: Destroy finished successfully", d.Id())

	d.SetId("")
	return err
}

func RestGet(d *schema.ResourceData, m interface{}) (*container.Container, error) {
	aciClient := m.(*client.Client)
	path := getPath(d.Get("dn").(string))

	req, err := aciClient.MakeRestRequest("GET", path, nil, true)
	if err != nil {
		return nil, err
	}

	respCont, _, err := aciClient.Do(req)
	if err != nil {
		return respCont, err
	}

	if respCont.S("imdata").Index(0).String() == "{}" {
		return nil, nil
	}

	err = client.CheckForErrors(respCont, "GET", false)
	if err != nil {
		return respCont, err
	}
	return respCont, nil
}

func RestPost(d *schema.ResourceData, m interface{}) (*container.Container, error) {
	aciClient := m.(*client.Client)
	path := getPath(d.Get("dn").(string))
	var cont *container.Container
	var err error
	method := "POST"

	content := d.Get("content")
	contentStrMap := toStrMap(content.(map[string]interface{}))

	className := d.Get("class_name").(string)
	cont, err = preparePayload(className, contentStrMap)
	if err != nil {
		return nil, err
	}

	req, err := aciClient.MakeRestRequest(method, path, cont, true)
	if err != nil {
		return nil, err
	}

	respCont, _, err := aciClient.Do(req)
	if err != nil {
		return respCont, err
	}
	err = client.CheckForErrors(respCont, method, false)
	if err != nil {
		return respCont, err
	}
	return cont, nil
}
