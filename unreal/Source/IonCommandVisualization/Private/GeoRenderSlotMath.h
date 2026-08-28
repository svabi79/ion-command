#pragma once

#include "CoreMinimal.h"

// Shared arithmetic for the swap-and-pop slot bookkeeping used by the arc
// and point layer renderers. Both own a dense (hole-free) run of fixed-size
// "blocks" inside a growable render-instance buffer (one
// UInstancedStaticMeshComponent per palette entry for arcs, one shared pool
// for points; a point's block size is 1, an arc's is SegmentsPerArc).
//
// Removing a block in the middle of a dense run would either leave a hole
// or force shifting every following block down by one - the exact O(N)
// rebuild cost this refactor exists to remove. Swap-and-pop avoids both: the
// LAST block's payload is copied into the freed block's position and the
// (now-duplicated) tail is trimmed. This keeps the run dense with a cost of
// O(BlockSize) instead of O(NumBlocks - FreedBlock), independent of how many
// blocks survive.
//
// This header only decides WHICH block indices are involved; it never
// touches a UInstancedStaticMeshComponent or any owner bookkeeping itself.
// The caller (GeoArcLayerActor / GeoPointLayerActor) owns the actual
// transform/custom-data copy and the owner-index maps, which keeps this
// logic small enough to unit-test exhaustively without spinning up a world,
// an actor, or a render component - see GeoRenderSlotMathTests.cpp.
namespace GeoRenderSlotMath
{
    // Decides what must move when freeing block `FreedBlock` out of
    // `BlockCountBeforeFree` dense, equally-sized blocks so the run stays
    // hole-free afterwards.
    //
    // Returns true and sets OutMovedFromBlock to the PREVIOUS index of the
    // block that must now be copied into FreedBlock's position (its new
    // index is always FreedBlock) when a move is required.
    //
    // Returns false when FreedBlock was already the last block: nothing
    // needs to move.
    //
    // Either way, the caller must afterwards trim the trailing block (the
    // one at BlockCountBeforeFree - 1), which is now either a stale
    // duplicate (move case) or the freed block itself (no-move case).
    inline bool ComputeFreeMove(int32 FreedBlock, int32 BlockCountBeforeFree, int32& OutMovedFromBlock)
    {
        check(BlockCountBeforeFree > 0);
        check(FreedBlock >= 0 && FreedBlock < BlockCountBeforeFree);
        const int32 LastBlock = BlockCountBeforeFree - 1;
        if (FreedBlock != LastBlock)
        {
            OutMovedFromBlock = LastBlock;
            return true;
        }
        return false;
    }
}
