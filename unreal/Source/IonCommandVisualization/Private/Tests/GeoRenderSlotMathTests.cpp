#include "GeoRenderSlotMath.h"
#include "Misc/AutomationTest.h"

#if WITH_DEV_AUTOMATION_TESTS

// These tests exercise ONLY the index arithmetic in GeoRenderSlotMath.h -
// the highest-risk part of the arc/point layer refactor (GeoArcLayerActor,
// GeoPointLayerActor): an off-by-one here would make a render slot point at
// the wrong owner, which shows up as an arc/marker that never disappears,
// disappears at random, or wears another item's colour. They deliberately
// do not touch UInstancedStaticMeshComponent, an Actor, or a UWorld, so
// they run instantly and cannot be confused by any other engine state.
//
// Each test drives a real FGeoInstanceSlotOwners shadow structure (defined
// below) using ONLY the same primitive GeoRenderSlotMath::ComputeFreeMove
// gives the real renderers, and checks the dense/1:1 invariant after every
// single operation - not just at the end.

namespace
{
    // Minimal "owner of each block" shadow, standing in for what
    // GeoArcLayerActor (ArcsByPalette) and GeoPointLayerActor
    // (ActivePoints/SlotToPointIndex) each do for real: a dense array of
    // blocks, plus a reverse map from a stable item id to its current block.
    // This is test-only scaffolding - production code never uses it - built
    // specifically so the test can assert the invariant a real renderer
    // relies on: after ComputeFreeMove's guidance is applied, every live
    // id's recorded block still points at the block that actually holds it.
    struct FGeoInstanceSlotOwners
    {
        TArray<int32> BlockOwnerId;    // BlockOwnerId[Block] = item id occupying Block
        TMap<int32, int32> IdToBlock;  // inverse of the above

        int32 Allocate(int32 Id)
        {
            const int32 Block = BlockOwnerId.Add(Id);
            IdToBlock.Add(Id, Block);
            return Block;
        }

        // Mirrors exactly what GeoArcLayerActor::RemoveArcAtSlot and
        // GeoPointLayerActor::RemoveRenderInstance do: ask
        // GeoRenderSlotMath for the move, apply it to the shadow arrays,
        // then trim the tail - never touching any block below the tail.
        void Free(int32 Block)
        {
            const int32 CountBefore = BlockOwnerId.Num();
            // Capture who is actually leaving BEFORE any mutation: unlike
            // the real renderers (which already know the departing item
            // from the caller's own reference/index and never need to read
            // it back out of the block array), this shadow's Free() only
            // takes a block number, so it must look the id up itself - and
            // must do so before BlockOwnerId[Block] is possibly overwritten
            // by the move below.
            const int32 FreedId = BlockOwnerId[Block];
            int32 MovedFromBlock = INDEX_NONE;
            if (GeoRenderSlotMath::ComputeFreeMove(Block, CountBefore, MovedFromBlock))
            {
                const int32 MovedId = BlockOwnerId[MovedFromBlock];
                BlockOwnerId[Block] = MovedId;
                IdToBlock[MovedId] = Block;
            }
            IdToBlock.Remove(FreedId);
            BlockOwnerId.SetNum(CountBefore - 1, EAllowShrinking::No);
        }

        int32 Num() const { return BlockOwnerId.Num(); }
    };

    // Verifies the two-way invariant a real renderer depends on: every
    // recorded (id, block) pair is consistent in both directions, and the
    // owner array is exactly as long as the id count (dense, no holes).
    bool CheckInvariant(FAutomationTestBase& Test, const FGeoInstanceSlotOwners& Owners, const TSet<int32>& ExpectedLiveIds, const TCHAR* Context)
    {
        bool bOk = true;
        bOk &= Test.TestEqual(FString::Printf(TEXT("%s: block count matches live id count"), Context), Owners.Num(), ExpectedLiveIds.Num());
        bOk &= Test.TestEqual(FString::Printf(TEXT("%s: reverse map size matches live id count"), Context), Owners.IdToBlock.Num(), ExpectedLiveIds.Num());
        for (int32 Id : ExpectedLiveIds)
        {
            const int32* Block = Owners.IdToBlock.Find(Id);
            if (!Test.TestNotNull(FString::Printf(TEXT("%s: id %d has a recorded block"), Context, Id), Block))
            {
                bOk = false;
                continue;
            }
            if (!Owners.BlockOwnerId.IsValidIndex(*Block))
            {
                Test.AddError(FString::Printf(TEXT("%s: id %d's recorded block %d is out of range [0,%d)"), Context, Id, *Block, Owners.Num()));
                bOk = false;
                continue;
            }
            bOk &= Test.TestEqual(FString::Printf(TEXT("%s: block %d's owner points back to id %d"), Context, *Block, Id), Owners.BlockOwnerId[*Block], Id);
        }
        return bOk;
    }
}

// Single alloc/free sequence, hand-traced: proves the basic swap-and-pop
// mechanics (move-into-freed-slot, no-move-when-already-last) match a
// worked-by-hand example before the fuzz test below trusts the pattern at
// scale.
IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoRenderSlotMathBasicTest, "IONCOMMAND.Visualization.SlotMath.BasicAllocateFree", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoRenderSlotMathBasicTest::RunTest(const FString& Parameters)
{
    FGeoInstanceSlotOwners Owners;
    for (int32 Id = 0; Id < 5; ++Id) Owners.Allocate(Id); // blocks 0..4 hold ids 0..4
    CheckInvariant(*this, Owners, {0, 1, 2, 3, 4}, TEXT("after 5 allocations"));

    // Free the middle block (2): the last block (id 4) must move into it.
    Owners.Free(2);
    TestEqual(TEXT("id 4 relocated into freed block 2"), Owners.IdToBlock[4], 2);
    TestFalse(TEXT("id 2 no longer tracked"), Owners.IdToBlock.Contains(2));
    CheckInvariant(*this, Owners, {0, 1, 3, 4}, TEXT("after freeing block 2 (mid-run)"));

    // Free the block that is already last: no relocation should occur.
    const int32 LastId = Owners.BlockOwnerId.Last();
    const int32 LastBlock = Owners.Num() - 1;
    Owners.Free(LastBlock);
    TestFalse(TEXT("last id no longer tracked after freeing the true tail"), Owners.IdToBlock.Contains(LastId));
    TSet<int32> RemainingIds;
    for (int32 Id : Owners.BlockOwnerId) RemainingIds.Add(Id);
    CheckInvariant(*this, Owners, RemainingIds, TEXT("after freeing the true tail"));

    return true;
}

// Proves the specific pattern GeoArcLayerActor::TrimToCapacity and
// GeoPointLayerActor::TrimToCapacity both rely on: selecting an arbitrary
// batch of victims and freeing them in DESCENDING block order removes
// exactly the intended set, because a not-yet-processed victim's block can
// never be the one a later-processed victim's removal relocates (every
// swap source is the current last block, which is always numerically above
// every remaining victim once processed high-to-low).
IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoRenderSlotMathDescendingBatchRemovalTest, "IONCOMMAND.Visualization.SlotMath.DescendingBatchRemovalIsExact", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoRenderSlotMathDescendingBatchRemovalTest::RunTest(const FString& Parameters)
{
    FRandomStream Rng(20260828);
    for (int32 Trial = 0; Trial < 500; ++Trial)
    {
        const int32 TotalCount = Rng.RandRange(1, 40);
        FGeoInstanceSlotOwners Owners;
        TSet<int32> AllIds;
        for (int32 Id = 0; Id < TotalCount; ++Id) { Owners.Allocate(Id); AllIds.Add(Id); }

        // Choose a random victim subset by BLOCK index (== id, at this
        // point, since nothing has moved yet), sized 0..TotalCount.
        const int32 VictimCount = Rng.RandRange(0, TotalCount);
        TSet<int32> VictimBlocks;
        while (VictimBlocks.Num() < VictimCount) VictimBlocks.Add(Rng.RandRange(0, TotalCount - 1));

        TArray<int32> Sorted = VictimBlocks.Array();
        Sorted.Sort(TGreater<int32>()); // descending, as TrimToCapacity does
        for (int32 Block : Sorted) Owners.Free(Block);

        TSet<int32> ExpectedSurvivors = AllIds.Difference(VictimBlocks); // ids == original blocks here
        if (!CheckInvariant(*this, Owners, ExpectedSurvivors, *FString::Printf(TEXT("trial %d (total=%d, victims=%d)"), Trial, TotalCount, VictimCount)))
        {
            return false; // stop at the first failing trial instead of flooding the log
        }
    }
    return true;
}

// Large-scale random alloc/free sequence (single operations, interleaved),
// checking the invariant after EVERY operation rather than only at the
// end - this is what would catch a bug that self-heals over a few
// subsequent operations but is wrong at the moment it happens.
IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoRenderSlotMathRandomOpsStayConsistentTest, "IONCOMMAND.Visualization.SlotMath.RandomOpsStayConsistent", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoRenderSlotMathRandomOpsStayConsistentTest::RunTest(const FString& Parameters)
{
    FGeoInstanceSlotOwners Owners;
    TArray<int32> LiveIds;
    TSet<int32> LiveIdSet;
    int32 NextId = 0;
    FRandomStream Rng(424242);

    constexpr int32 OperationCount = 20000;
    for (int32 Op = 0; Op < OperationCount; ++Op)
    {
        const bool bAllocate = LiveIds.IsEmpty() || Rng.FRand() < 0.55;
        if (bAllocate)
        {
            const int32 Id = NextId++;
            Owners.Allocate(Id);
            LiveIds.Add(Id);
            LiveIdSet.Add(Id);
        }
        else
        {
            const int32 VictimListIndex = Rng.RandRange(0, LiveIds.Num() - 1);
            const int32 VictimId = LiveIds[VictimListIndex];
            Owners.Free(Owners.IdToBlock[VictimId]);
            LiveIds.RemoveAtSwap(VictimListIndex);
            LiveIdSet.Remove(VictimId);
        }
        // Checking every single step (not just periodically) is the point:
        // a drift that lasts only one operation before something else masks
        // it would otherwise slip through.
        if (!CheckInvariant(*this, Owners, LiveIdSet, *FString::Printf(TEXT("after op %d (%s)"), Op, bAllocate ? TEXT("allocate") : TEXT("free"))))
        {
            return false;
        }
    }
    TestEqual(TEXT("final live count matches shadow bookkeeping"), Owners.Num(), LiveIds.Num());
    return true;
}

// Block size only changes how many render instances move per operation
// (SegmentsPerArc for arcs, 1 for points) - it never changes the index
// arithmetic itself, which operates purely on block numbers. This test
// exists to document and pin that independence explicitly, exercising a
// non-trivial block size (matching GeoArcLayerActor's default
// SegmentsPerArc) through the same allocate/free/verify pattern.
IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoRenderSlotMathBlockSizeIndependenceTest, "IONCOMMAND.Visualization.SlotMath.BlockSizeIndependence", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoRenderSlotMathBlockSizeIndependenceTest::RunTest(const FString& Parameters)
{
    constexpr int32 SegmentsPerArc = 16;
    FGeoInstanceSlotOwners Owners; // tracks BLOCKS; instance range is Block*SegmentsPerArc..+SegmentsPerArc
    TSet<int32> LiveIds;
    for (int32 Id = 0; Id < 200; ++Id) { Owners.Allocate(Id); LiveIds.Add(Id); }

    FRandomStream Rng(160816);
    for (int32 Op = 0; Op < 5000; ++Op)
    {
        if (LiveIds.Num() > 1 && Rng.FRand() < 0.5)
        {
            const int32 VictimId = LiveIds.Array()[Rng.RandRange(0, LiveIds.Num() - 1)];
            const int32 Block = Owners.IdToBlock[VictimId];
            // What a real caller would do with the block size: derive the
            // instance range about to be touched/trimmed. Checked here only
            // for range validity, since this test's shadow does not model
            // an actual instance buffer.
            const int32 FirstInstance = Block * SegmentsPerArc;
            TestTrue(TEXT("derived instance range starts in bounds"), FirstInstance >= 0 && FirstInstance < Owners.Num() * SegmentsPerArc);
            Owners.Free(Block);
            LiveIds.Remove(VictimId);
        }
        else
        {
            const int32 Id = 1000000 + Op;
            Owners.Allocate(Id);
            LiveIds.Add(Id);
        }
    }
    return CheckInvariant(*this, Owners, LiveIds, TEXT("after mixed block-size-aware ops"));
}

#endif
