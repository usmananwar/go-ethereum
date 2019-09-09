package cliquee

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

var (
	stakingMap         map[common.Address]uint64
	stakersList        []common.Address
	totalStakingAmount uint64
	rangeTable         map[common.Address]Range
	matrix             map[common.Address]int
)

type Range struct {
	low  float64
	high float64
}

type StakersListAscending []common.Address

func (sa StakersListAscending) Len() int { return len(sa) }
func (sa StakersListAscending) Less(i, j int) bool {

	if stakingMap[sa[i]] > stakingMap[sa[j]] {
		return true
	}
	return false
}
func (sa StakersListAscending) Swap(i, j int) { sa[i], sa[j] = sa[j], sa[i] }

// isAuthorized
func isAuthorized(blockNumber uint64, staker common.Address, distribution map[common.Address]Range) {
	min := float64(0)
	max := float64(100)
	randomSource := rand.New(rand.NewSource(int64(blockNumber)))
	randomNumber := randomSource.Float64()*(max-min) + min
	stakerRange := distribution[staker]

	fmt.Println(" random number: ", randomNumber)
	if randomNumber >= stakerRange.low && randomNumber < stakerRange.high {

		fmt.Print("low: ", stakerRange.low)
		fmt.Print(" high: ", stakerRange.high)

		fmt.Println("TRUE")
	}
}

// isAuthorized
func findAuthorizedNode(blockNumber uint64, distribution map[common.Address]*Range) {
	min := float64(0)
	max := float64(100)
	randomSource := rand.New(rand.NewSource(int64(blockNumber)))
	randomNumber := randomSource.Float64()*(max-min) + min

	//fmt.Println(" Random number: ", randomNumber)

	for staker := range distribution {
		if randomNumber >= distribution[staker].low && randomNumber < distribution[staker].high {
			//fmt.Println("Authorized staker: ", hex.EncodeToString(staker.Bytes()), " for Block#: ", blockNumber)
			matrix[staker] = matrix[staker] + 1
		}
	}

}

func findStakerIndex(staker common.Address) int {
	for index, element := range stakersList {
		if element == staker {
			return index
		}
	}
	return 0
}

func calculateWeightageTable() map[common.Address]*float64 {
	weightageMap := make(map[common.Address]*float64)
	for _, staker := range stakersList {
		temp := (float64(stakingMap[staker]) / float64(totalStakingAmount)) * 100
		weightageMap[staker] = &temp
		fmt.Println("Staker: ", hex.EncodeToString(staker.Bytes()), " Weightage: ", temp)
	}
	return weightageMap
}

func getDistributedWeightage(weightageMap map[common.Address]*float64) map[common.Address]*Range {
	distributedWeightage := make(map[common.Address]*Range)
	low := float64(0)
	for _, staker := range stakersList {
		//fmt.Print("low: ", low)
		high := low + *weightageMap[staker]
		//fmt.Println(" high: ", low+*weightageMap[staker])
		distributedWeightage[staker] = &Range{
			low:  low,
			high: high,
		}
		low = high
	}
	return distributedWeightage
}

//TestClique asdfasdf
func TestGest(t *testing.T) {

	matrix = make(map[common.Address]int)

	populateStakersList()
	sortNprint()
	weightageMap := calculateWeightageTable()
	distribution := getDistributedWeightage(weightageMap)
	//isAuthorized(360, common.HexToAddress("78c2b0dfde452677ccd0cd00465e7cca0e3c5350"), distribution)

	for i := uint64(0); i < 200; i++ {
		findAuthorizedNode(i, distribution)
	}

	for staker := range matrix {
		fmt.Println("Miner: ", hex.EncodeToString(staker.Bytes()), "  Number of blocks:	", matrix[staker], "	With weightage: ", *weightageMap[staker])
	}

}

func sortNprint() {
	sort.Sort(StakersListAscending(stakersList))
	sum := uint64(0)
	for _, element := range stakersList {
		fmt.Println(stakingMap[element])
		sum = sum + stakingMap[element]
	}

	fmt.Println("Total staking amount is: ", sum)

	totalStakingAmount = sum
}

func populateStakersList() {

	stakingMap = make(map[common.Address]uint64)
	stakingMap[common.HexToAddress("0x71c2b0dfde452677ccd0cd00465e7cca0e3c5353")] = 10
	stakingMap[common.HexToAddress("0x72c2b0dfde452677ccd0cd00465e7cca0e3c5354")] = 23
	stakingMap[common.HexToAddress("0x73c2b0dfde452677ccd0cd00465e7cca0e3c5351")] = 12
	stakingMap[common.HexToAddress("0x74c2b0dfde452677ccd0cd00465e7cca0e3c5352")] = 15
	stakingMap[common.HexToAddress("0x75c2b0dfde452677ccd0cd00465e7cca0e3c5356")] = 16
	stakingMap[common.HexToAddress("0x76c2b0dfde452677ccd0cd00465e7cca0e3c5357")] = 107
	stakingMap[common.HexToAddress("0x77c2b0dfde452677ccd0cd00465e7cca0e3c5350")] = 106

	stakersList = make([]common.Address, 0, len(stakingMap))

	for staker := range stakingMap {
		stakersList = append(stakersList, staker)
		fmt.Print(hex.EncodeToString(staker.Bytes()))
		fmt.Println(": ", stakingMap[staker])
	}

}
