package clique

import (
	"github.com/ethereum/go-ethereum/common"
	"math/rand"
)

type StakersListAscending []common.Address

func (sa StakersListAscending) Len() int { return len(sa) }
func (sa StakersListAscending) Less(i, j int) bool {

	if stakingMap[sa[i]] > stakingMap[sa[j]] {
		return true
	}
	return false
}
func (sa StakersListAscending) Swap(i, j int) { sa[i], sa[j] = sa[j], sa[i] }

func getARandomNumber(seed, min, max float64) float64 {
	randomSource := rand.New(rand.NewSource(int64(seed)))
	randomNumber := randomSource.Float64()*(max-min) + min
	return randomNumber
}
