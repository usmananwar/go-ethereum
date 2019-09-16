// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package clique

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"sort"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	lru "github.com/hashicorp/golang-lru"
)

// BerithSnapshot is the state of the authorization voting at a given point in time.
type BerithSnapshot struct {
	config *params.CliqueConfig // Consensus engine parameters to fine tune behavior

	Number            uint64                     `json:"number"`      // Block number where the snapshot was created
	Hash              common.Hash                `json:"hash"`        // Block hash where the snapshot was created
	StakingMap        map[common.Address]uint64  `json:"staking_map"` // Set of authorized signers at this moment
	StakersList       []Staker                   `json:"stakers"`
	WeightageMap      map[common.Address]float64 `json:"weightage"`
	Distribution      map[common.Address]Range   `json:"distribution"`
	TotalStakedAmount uint64                     `json:"total_staked_amount"`
}

// Range of distribution
type Range struct {
	low  float64
	high float64
}

// Staker of distribution
type Staker struct {
	address      common.Address
	stakedAmount uint64
}

// NewBerithSnapshot creates a new snapshot with the specified startup parameters. This
// method does not initialize the set of recent signers, so only ever use if for
// the genesis block.
func NewBerithSnapshot(config *params.CliqueConfig, sigcache *lru.ARCCache, number uint64, hash common.Hash, stakingMap map[common.Address]uint64) *BerithSnapshot {
	snap := &BerithSnapshot{
		config:            config,
		Number:            number,
		Hash:              hash,
		StakingMap:        stakingMap,
		StakersList:       make([]Staker, 0, len(stakingMap)),
		WeightageMap:      make(map[common.Address]float64),
		Distribution:      make(map[common.Address]Range),
		TotalStakedAmount: 0,
	}

	for staker := range snap.StakingMap {
		snap.StakersList = append(snap.StakersList, Staker{
			address:      staker,
			stakedAmount: snap.StakingMap[staker],
		})
		snap.TotalStakedAmount = snap.TotalStakedAmount + snap.StakingMap[staker]
	}

	sort.Sort(StakersListAscending(snap.StakersList)) // Sorting the stakers list*/
	calculateWeightageMap(snap)
	calculateDistributedWeightage(snap)

	log.Info("Sorter staking list")
	for _, listEntry := range snap.StakersList {
		log.Info("DEBUGGING", "staker", hex.EncodeToString(listEntry.address.Bytes()), "weightage", snap.WeightageMap[listEntry.address], "distributed", snap.Distribution[listEntry.address])
	}
	log.Info("DEBUGGING", "Total staked amount", snap.TotalStakedAmount)

	return snap
}

func calculateWeightageMap(snap *BerithSnapshot) {
	for _, staker := range snap.StakersList {
		temp := (float64(staker.stakedAmount) / float64(snap.TotalStakedAmount)) * 100
		snap.WeightageMap[staker.address] = temp
	}
}

func calculateDistributedWeightage(snap *BerithSnapshot) {
	low := float64(0)
	for _, staker := range snap.StakersList {
		high := low + snap.WeightageMap[staker.address]
		snap.Distribution[staker.address] = Range{
			low:  low,
			high: high,
		}
		low = high
	}
}

// store inserts the snapshot into the database.
func (s *BerithSnapshot) store(db ethdb.Database) error {
	blob, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return db.Put(append([]byte("berith-"), s.Hash[:]...), blob)
}

// loadBerithSnapshot loads an existing snapshot from the database.
func loadBerithSnapshot(config *params.CliqueConfig, sigcache *lru.ARCCache, db ethdb.Database, hash common.Hash) (*BerithSnapshot, error) {
	blob, err := db.Get(append([]byte("berith-"), hash[:]...))
	if err != nil {
		return nil, err
	}
	snap := new(BerithSnapshot)
	if err := json.Unmarshal(blob, snap); err != nil {
		return nil, err
	}
	snap.config = config
	//snap.sigcache = sigcache

	return snap, nil
}

// signers retrieves the list of authorized signers in ascending order.
func (s *BerithSnapshot) signers() []common.Address {
	sigs := make([]common.Address, 0, len(s.StakersList))
	for _, sig := range s.StakersList {
		sigs = append(sigs, sig.address)
	}
	sort.Sort(signersAscending(sigs))
	return sigs
}

func (s *BerithSnapshot) isOneOfRandomSealers(blockNumber uint64, signer common.Address) bool {
	seed := getSeedFromNumber(blockNumber)
	for i := float64(0); i < 0; i++ { // Number of allowed forks
		randNumber := getARandomNumber(seed+i, 0, float64(len(s.StakersList)-1))
		index := int(math.Round(randNumber))
		fmt.Print(seed+i, " : ")
		fmt.Println(index)
		if s.StakersList[index].address == signer {
			return true
		}
	}
	return false
}

func getSeedFromNumber(number uint64) float64 {
	s := string(number)
	h := sha1.New()
	h.Write([]byte(s))
	hexForm := hex.EncodeToString(h.Sum(nil))
	splitted := hexForm[:len(hexForm)-(len(hexForm)-5)]

	fmt.Print(splitted)

	i := new(big.Int)
	i.SetString(splitted, 16)
	return float64(i.Int64())
}

func (s *BerithSnapshot) isInTurn(blockNumber uint64, staker common.Address) bool {
	min := float64(0)
	max := float64(100)
	randomSource := rand.New(rand.NewSource(int64(blockNumber)))
	randomNumber := randomSource.Float64()*(max-min) + min
	stakerRange := s.Distribution[staker]

	log.Info("Method: isInTurn", "random#", randomNumber, "staker", staker, "found range", stakerRange)
	if randomNumber >= stakerRange.low && randomNumber < stakerRange.high {
		log.Info("Method: isInTurn", "low", stakerRange.low)
		log.Info("Method: isInTurn", "high", stakerRange.high)
		return true
	}

	return false
}

// StakersListAscending asdf
type StakersListAscending []Staker

func (sa StakersListAscending) Len() int { return len(sa) }
func (sa StakersListAscending) Less(i, j int) bool {
	if sa[i].stakedAmount > sa[j].stakedAmount {
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
