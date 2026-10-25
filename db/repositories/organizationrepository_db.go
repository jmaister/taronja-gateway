package repositories

import (
	"errors"

	"github.com/jmaister/taronja-gateway/db"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OrganizationRepositoryDB implements OrganizationRepository using GORM.
type OrganizationRepositoryDB struct {
	db *gorm.DB
}

// NewOrganizationRepositoryDB creates a new OrganizationRepositoryDB.
func NewOrganizationRepositoryDB(db *gorm.DB) *OrganizationRepositoryDB {
	return &OrganizationRepositoryDB{db: db}
}

// var _ OrganizationRepository = (*OrganizationRepositoryDB)(nil) ensures
// the type satisfies the interface at compile time.
var _ OrganizationRepository = (*OrganizationRepositoryDB)(nil)

// Create creates a new organization.
func (r *OrganizationRepositoryDB) Create(org *db.Organization) error {
	return r.db.Create(org).Error
}

// Get retrieves an organization by id, returning gorm.ErrRecordNotFound if not found.
func (r *OrganizationRepositoryDB) Get(id string) (*db.Organization, error) {
	var org db.Organization
	err := r.db.Where("id = ?", id).First(&org).Error
	if err != nil {
		return nil, err
	}
	return &org, nil
}

// List returns all organizations.
func (r *OrganizationRepositoryDB) List() ([]db.Organization, error) {
	var orgs []db.Organization
	err := r.db.Find(&orgs).Error
	return orgs, err
}

// Update updates an existing organization.
func (r *OrganizationRepositoryDB) Update(org *db.Organization) error {
	return r.db.Save(org).Error
}

// Delete removes the organization and all its memberships in a transaction.
func (r *OrganizationRepositoryDB) Delete(id string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// First, delete all members
		err := tx.Where("organization_id = ?", id).Delete(&db.OrganizationMember{}).Error
		if err != nil {
			return err
		}
		// Then delete the organization
		result := tx.Where("id = ?", id).Delete(&db.Organization{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// SetMember adds userID to the organization with the given role, or updates
// the role if the member already exists. Returns gorm.ErrRecordNotFound if
// the organization does not exist.
func (r *OrganizationRepositoryDB) SetMember(orgID, userID, role string) error {
	// Verify organization exists
	var org db.Organization
	err := r.db.Where("id = ?", orgID).First(&org).Error
	if err != nil {
		return err
	}

	// Upsert the member: try to create, on duplicate constraint, update the role
	member := db.OrganizationMember{
		OrganizationID: orgID,
		UserID:         userID,
		Role:           role,
	}

	// Using clauses to handle upsert (INSERT ... ON CONFLICT)
	return r.db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Create(&member).Error
}

// RemoveMember removes a user from an organization, returning
// gorm.ErrRecordNotFound if the membership does not exist.
func (r *OrganizationRepositoryDB) RemoveMember(orgID, userID string) error {
	result := r.db.Where("organization_id = ? AND user_id = ?", orgID, userID).Delete(&db.OrganizationMember{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Members returns all members of an organization.
func (r *OrganizationRepositoryDB) Members(orgID string) ([]db.OrganizationMember, error) {
	var members []db.OrganizationMember
	err := r.db.Where("organization_id = ?", orgID).Order("created_at ASC").Find(&members).Error
	return members, err
}

// Member returns the membership record for a specific user in an organization,
// or gorm.ErrRecordNotFound if not found.
func (r *OrganizationRepositoryDB) Member(orgID, userID string) (*db.OrganizationMember, error) {
	var member db.OrganizationMember
	err := r.db.Where("organization_id = ? AND user_id = ?", orgID, userID).First(&member).Error
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// OfUser lists all organizations a user is a member of, ordered by name.
func (r *OrganizationRepositoryDB) OfUser(userID string) ([]OrganizationMembership, error) {
	var members []db.OrganizationMember

	// First get all memberships for this user
	err := r.db.Where("user_id = ?", userID).Find(&members).Error
	if err != nil {
		return nil, err
	}

	// Build the result by fetching each organization and combining with role
	var memberships []OrganizationMembership
	for _, member := range members {
		var org db.Organization
		err := r.db.Where("id = ?", member.OrganizationID).First(&org).Error
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
			// Skip if organization was deleted (shouldn't happen with proper FK constraints)
			continue
		}
		memberships = append(memberships, OrganizationMembership{
			Organization: org,
			Role:         member.Role,
		})
	}

	// Sort by organization name
	for i := 0; i < len(memberships); i++ {
		for j := i + 1; j < len(memberships); j++ {
			if memberships[j].Organization.Name < memberships[i].Organization.Name {
				memberships[i], memberships[j] = memberships[j], memberships[i]
			}
		}
	}

	return memberships, nil
}
