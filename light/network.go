package light

const (
	//VoteAgreeAgainst vote sign with against
	VoteAgreeAgainst = iota
	//VoteAgree vote sign with agree
	VoteAgree

	Normal = iota
	DiscTooManyPeers
	DownloaderCall
	ServerSimpleCall
	ServerSignedCall
	FetcherSyncCall
	FetcherHeadCall
	FetcherKnownCall
	FetcherTimerCall
	FetcherDeliverCall
	FetcherAnnounceCall
)
