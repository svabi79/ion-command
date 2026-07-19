#pragma once

#include "CoreMinimal.h"
#include "GameFramework/Actor.h"
#include "GeoTypes.h"
#include "IonActivityHeatmapActor.generated.h"

class UGeoDataSubsystem;
class UInstancedStaticMeshComponent;

// Density overlay of relationship endpoints on the globe: a fixed lat/lon
// grid of decaying counters rendered as soft additive splats hugging the
// surface. Hidden by default; the operator toggles it.
UCLASS()
class IONCOMMANDVISUALIZATION_API AIonActivityHeatmapActor final : public AActor
{
    GENERATED_BODY()

public:
    AIonActivityHeatmapActor();
    virtual void BeginPlay() override;
    virtual void EndPlay(const EEndPlayReason::Type EndPlayReason) override;
    virtual void Tick(float DeltaSeconds) override;

    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double GlobeRadius = 1000.0;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double SurfaceOffset = 6.0;
    // Per-second retention of a cell's heat (~45 s half-life at 0.985).
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double DecayPerSecond = 0.985;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") int32 MaxSplats = 900;

private:
    static constexpr int32 LonCells = 72;
    static constexpr int32 LatCells = 36;

    void OnMessageAccepted(const FGeoMessageEnvelope& Message);
    void OnDataReset();
    void AccumulatePosition(const FGeoPosition& Position);
    void RebuildSplats();

    UPROPERTY(VisibleAnywhere) TObjectPtr<USceneComponent> SceneRoot;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UInstancedStaticMeshComponent> Splats;
    TWeakObjectPtr<UGeoDataSubsystem> DataSubsystem;
    TArray<float> Heat;
    double LastRebuildSeconds = 0.0;
};
