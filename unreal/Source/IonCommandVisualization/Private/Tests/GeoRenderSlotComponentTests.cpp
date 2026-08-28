#include "Components/InstancedStaticMeshComponent.h"
#include "GeoRenderSlotMath.h"
#include "Misc/AutomationTest.h"
#include "UObject/Package.h"

#if WITH_DEV_AUTOMATION_TESTS

// GeoRenderSlotMathTests.cpp proves the block-index arithmetic in isolation.
// These tests drive the SAME swap-and-pop sequence against a REAL
// UInstancedStaticMeshComponent - closing the gap between "the algorithm is
// right" and "GeoArcLayerActor/GeoPointLayerActor call the real engine API
// correctly" (BatchUpdateInstancesTransforms, RemoveInstances,
// UpdateInstanceTransform, RemoveInstance, SetCustomDataValue/SetCustomData)
// without needing an Actor or a UWorld. NewObject<UInstancedStaticMeshComponent>
// on the transient package is the same construction pattern the engine's own
// System.Engine.InstancedStaticMesh test suite uses
// (Engine/Private/Tests/InstancedStaticMeshTest.cpp).

namespace
{
    UInstancedStaticMeshComponent* MakeTestComponent(int32 NumCustomDataFloats)
    {
        UInstancedStaticMeshComponent* ISM = NewObject<UInstancedStaticMeshComponent>(GetTransientPackage());
        ISM->SetNumCustomDataFloats(NumCustomDataFloats);
        return ISM;
    }

    // One custom-data float, holding a distinct "identity" tag per instance
    // so a test can tell after a swap whether the right payload moved.
    float ReadIdentity(const UInstancedStaticMeshComponent* ISM, int32 InstanceIndex)
    {
        return ISM->PerInstanceSMCustomData[InstanceIndex * ISM->NumCustomDataFloats];
    }

    FVector ReadLocation(const UInstancedStaticMeshComponent* ISM, int32 InstanceIndex)
    {
        FTransform Transform;
        ISM->GetInstanceTransform(InstanceIndex, Transform, true);
        return Transform.GetLocation();
    }
}

// Point-layer-shaped case: block size 1. Mirrors
// AGeoPointLayerActor::RemoveRenderInstance exactly - read the last
// instance's data, write it into the freed slot with UpdateInstanceTransform
// + SetCustomDataValue, then RemoveInstance() the (now-duplicated) last one.
IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoRenderSlotComponentSingleInstanceRemovalTest, "IONCOMMAND.Visualization.SlotMath.ComponentSingleInstanceSwapRemove", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoRenderSlotComponentSingleInstanceRemovalTest::RunTest(const FString& Parameters)
{
    UInstancedStaticMeshComponent* ISM = MakeTestComponent(1);
    constexpr int32 Count = 10;
    for (int32 Id = 0; Id < Count; ++Id)
    {
        const int32 Index = ISM->AddInstance(FTransform(FQuat::Identity, FVector(static_cast<double>(Id), 0, 0)), true);
        TestEqual(TEXT("instances append at the tail"), Index, Id);
        ISM->SetCustomDataValue(Index, 0, static_cast<float>(Id), false);
    }
    TestEqual(TEXT("initial instance count"), ISM->GetInstanceCount(), Count);

    // Remove id 3 (a middle slot): id 9 (the true tail) must relocate there.
    const int32 SlotToFree = 3;
    const int32 CountBeforeFree = ISM->GetInstanceCount();
    int32 MovedFromSlot = INDEX_NONE;
    const bool bMoved = GeoRenderSlotMath::ComputeFreeMove(SlotToFree, CountBeforeFree, MovedFromSlot);
    TestTrue(TEXT("freeing a middle slot requires a move"), bMoved);
    TestEqual(TEXT("move source is the true tail"), MovedFromSlot, CountBeforeFree - 1);

    const FVector MovedLocation = ReadLocation(ISM, MovedFromSlot);
    const float MovedIdentity = ReadIdentity(ISM, MovedFromSlot);
    ISM->UpdateInstanceTransform(SlotToFree, FTransform(FQuat::Identity, MovedLocation), true, false, true);
    ISM->SetCustomDataValue(SlotToFree, 0, MovedIdentity, false);
    ISM->RemoveInstance(CountBeforeFree - 1);

    TestEqual(TEXT("instance count decreased by one"), ISM->GetInstanceCount(), Count - 1);
    TestEqual(TEXT("freed slot now holds the relocated identity"), ReadIdentity(ISM, SlotToFree), 9.0f);
    TestTrue(TEXT("freed slot now holds the relocated location"), ReadLocation(ISM, SlotToFree).Equals(FVector(9, 0, 0)));

    // Every OTHER instance must be completely untouched by the removal -
    // this is the actual claim the refactor makes: removing one item never
    // disturbs any other item's instance.
    for (int32 Id = 0; Id < Count - 1; ++Id)
    {
        if (Id == SlotToFree) continue;
        if (!TestEqual(*FString::Printf(TEXT("slot %d identity undisturbed"), Id), ReadIdentity(ISM, Id), static_cast<float>(Id)))
        {
            return false;
        }
        if (!TestTrue(*FString::Printf(TEXT("slot %d location undisturbed"), Id), ReadLocation(ISM, Id).Equals(FVector(static_cast<double>(Id), 0, 0))))
        {
            return false;
        }
    }
    return true;
}

// Removing the slot that is ALREADY the true tail must not move anything.
IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoRenderSlotComponentTailRemovalNoMoveTest, "IONCOMMAND.Visualization.SlotMath.ComponentTailRemovalNoMove", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoRenderSlotComponentTailRemovalNoMoveTest::RunTest(const FString& Parameters)
{
    UInstancedStaticMeshComponent* ISM = MakeTestComponent(1);
    for (int32 Id = 0; Id < 6; ++Id)
    {
        const int32 Index = ISM->AddInstance(FTransform(FQuat::Identity, FVector(static_cast<double>(Id), 0, 0)), true);
        ISM->SetCustomDataValue(Index, 0, static_cast<float>(Id), false);
    }
    const int32 CountBeforeFree = ISM->GetInstanceCount();
    const int32 TailSlot = CountBeforeFree - 1;
    int32 MovedFromSlot = INDEX_NONE;
    const bool bMoved = GeoRenderSlotMath::ComputeFreeMove(TailSlot, CountBeforeFree, MovedFromSlot);
    TestFalse(TEXT("freeing the true tail needs no move"), bMoved);
    ISM->RemoveInstance(TailSlot);
    TestEqual(TEXT("instance count decreased by one"), ISM->GetInstanceCount(), 5);
    for (int32 Id = 0; Id < 5; ++Id)
    {
        if (!TestEqual(*FString::Printf(TEXT("slot %d identity undisturbed"), Id), ReadIdentity(ISM, Id), static_cast<float>(Id)))
        {
            return false;
        }
    }
    return true;
}

// Arc-layer-shaped case: block size SegmentsPerArc (16), matching
// AGeoArcLayerActor::RemoveArcAtSlot - a batched BatchUpdateInstancesTransforms
// write for the relocated block's segments, then RemoveInstances() on the
// trailing SegmentsPerArc indices, descending-sorted.
IMPLEMENT_SIMPLE_AUTOMATION_TEST(FGeoRenderSlotComponentBlockRemovalTest, "IONCOMMAND.Visualization.SlotMath.ComponentBlockSwapRemove", EAutomationTestFlags::EditorContext | EAutomationTestFlags::EngineFilter)
bool FGeoRenderSlotComponentBlockRemovalTest::RunTest(const FString& Parameters)
{
    constexpr int32 SegmentsPerArc = 16;
    constexpr int32 BlockCount = 8;
    UInstancedStaticMeshComponent* ISM = MakeTestComponent(1);

    // Block B's segments are tagged with identity B and X = B*100 + segment,
    // so a misrouted segment (wrong block OR wrong segment-within-block) is
    // immediately visible in either check below.
    for (int32 Block = 0; Block < BlockCount; ++Block)
    {
        TArray<FTransform> Transforms;
        Transforms.Reserve(SegmentsPerArc);
        for (int32 Segment = 0; Segment < SegmentsPerArc; ++Segment)
        {
            Transforms.Emplace(FQuat::Identity, FVector(Block * 100.0 + Segment, 0, 0));
        }
        const TArray<int32> Indices = ISM->AddInstances(Transforms, true, true);
        TestEqual(TEXT("block appended contiguously"), Indices[0], Block * SegmentsPerArc);
        for (int32 Segment = 0; Segment < SegmentsPerArc; ++Segment)
        {
            ISM->SetCustomDataValue(Indices[Segment], 0, static_cast<float>(Block), false);
        }
    }
    TestEqual(TEXT("initial instance count"), ISM->GetInstanceCount(), BlockCount * SegmentsPerArc);

    // Free block 2 (a middle block): block 7 (the true tail block) relocates.
    const int32 BlockToFree = 2;
    const int32 BlockCountBeforeFree = BlockCount;
    int32 MovedFromBlock = INDEX_NONE;
    const bool bMoved = GeoRenderSlotMath::ComputeFreeMove(BlockToFree, BlockCountBeforeFree, MovedFromBlock);
    TestTrue(TEXT("freeing a middle block requires a move"), bMoved);
    TestEqual(TEXT("move source is the true tail block"), MovedFromBlock, BlockCountBeforeFree - 1);

    TArray<FTransform> MovedTransforms;
    MovedTransforms.Reserve(SegmentsPerArc);
    const int32 MovedBase = MovedFromBlock * SegmentsPerArc;
    for (int32 Segment = 0; Segment < SegmentsPerArc; ++Segment)
    {
        MovedTransforms.Add(FTransform(FQuat::Identity, ReadLocation(ISM, MovedBase + Segment)));
    }
    const int32 FreedBase = BlockToFree * SegmentsPerArc;
    ISM->BatchUpdateInstancesTransforms(FreedBase, MovedTransforms, /*bWorldSpace=*/true, /*bMarkRenderStateDirty=*/false, /*bTeleport=*/true);
    for (int32 Segment = 0; Segment < SegmentsPerArc; ++Segment)
    {
        ISM->SetCustomDataValue(FreedBase + Segment, 0, static_cast<float>(MovedFromBlock), false);
    }
    TArray<int32> ToRemove;
    ToRemove.Reserve(SegmentsPerArc);
    for (int32 Segment = SegmentsPerArc - 1; Segment >= 0; --Segment) ToRemove.Add(MovedBase + Segment);
    ISM->RemoveInstances(ToRemove, /*bInstanceArrayAlreadySortedInReverseOrder=*/true);

    TestEqual(TEXT("instance count decreased by one block"), ISM->GetInstanceCount(), (BlockCount - 1) * SegmentsPerArc);

    // The relocated block's segments must land in the freed block's range,
    // each segment keeping its own within-block position (segment 0 of the
    // moved block must not end up wearing segment 5's data, etc.).
    for (int32 Segment = 0; Segment < SegmentsPerArc; ++Segment)
    {
        if (!TestEqual(*FString::Printf(TEXT("relocated block identity, segment %d"), Segment), ReadIdentity(ISM, FreedBase + Segment), static_cast<float>(MovedFromBlock)))
        {
            return false;
        }
        const FVector Expected(MovedFromBlock * 100.0 + Segment, 0, 0);
        if (!TestTrue(*FString::Printf(TEXT("relocated block location, segment %d"), Segment), ReadLocation(ISM, FreedBase + Segment).Equals(Expected)))
        {
            return false;
        }
    }

    // Every untouched block (0,1,3,4,5,6 - everything except the freed
    // block 2 and the block that moved into it) must be bit-for-bit as
    // originally written.
    for (int32 Block = 0; Block < BlockCount - 1; ++Block)
    {
        if (Block == BlockToFree) continue;
        const int32 Base = Block * SegmentsPerArc;
        for (int32 Segment = 0; Segment < SegmentsPerArc; ++Segment)
        {
            if (!TestEqual(*FString::Printf(TEXT("block %d segment %d identity undisturbed"), Block, Segment), ReadIdentity(ISM, Base + Segment), static_cast<float>(Block)))
            {
                return false;
            }
            const FVector Expected(Block * 100.0 + Segment, 0, 0);
            if (!TestTrue(*FString::Printf(TEXT("block %d segment %d location undisturbed"), Block, Segment), ReadLocation(ISM, Base + Segment).Equals(Expected)))
            {
                return false;
            }
        }
    }
    return true;
}

#endif
