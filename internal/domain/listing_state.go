package domain

// OwnerChange is the fact the indexer observes on chain: an NFT item is now
// owned by NewOwner (raw address). Ownership on chain is the source of truth
// for listing state; nothing else moves a listing forward.
type OwnerChange struct {
	NFTAddress string
	NewOwner   string
}

// NextListingStatus is the listing state machine. It is a pure function so it
// can be unit-tested exhaustively and reused by the indexer and reconciler.
//
//	pending  + owner == sale contract -> active
//	pending  + owner == seller        -> pending (transfer not landed yet)
//	active   + owner == seller        -> cancelled
//	active   + owner != sale && != seller -> sold (buyer = owner)
//	terminal + anything               -> unchanged
//
// The boolean reports whether the status changed.
func NextListingStatus(l Listing, newOwner string) (ListingStatus, bool) {
	switch l.Status {
	case ListingPending:
		if newOwner == l.SaleContract {
			return ListingActive, true
		}
		if newOwner != "" && newOwner != l.Seller {
			// NFT moved somewhere else before the sale was initialised: the
			// listing can never become active.
			return ListingCancelled, true
		}
		return ListingPending, false
	case ListingActive:
		if newOwner == l.SaleContract {
			return ListingActive, false
		}
		if newOwner == l.Seller {
			return ListingCancelled, true
		}
		if newOwner != "" {
			return ListingSold, true
		}
		return ListingActive, false
	default:
		return l.Status, false
	}
}

// Apply applies an observed owner to the listing, returning the updated copy
// and whether anything changed. On a sale the buyer is recorded.
func (l Listing) Apply(newOwner string) (Listing, bool) {
	next, changed := NextListingStatus(l, newOwner)
	if !changed {
		return l, false
	}
	l.Status = next
	if next == ListingSold {
		l.Buyer = newOwner
	}
	return l, true
}

// TxTypeForOwnerChange infers which pending transaction type an owner change
// confirms for a given listing.
func TxTypeForOwnerChange(l Listing, newOwner string) (TxType, bool) {
	switch {
	case newOwner == l.SaleContract:
		return TxList, true
	case newOwner == l.Seller && l.Status == ListingActive:
		return TxCancel, true
	case newOwner != "" && newOwner != l.Seller && newOwner != l.SaleContract:
		return TxBuy, true
	}
	return "", false
}
