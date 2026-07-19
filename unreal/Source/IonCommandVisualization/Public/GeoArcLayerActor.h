#pragma once

#include "CoreMinimal.h"
#include "GameFramework/Actor.h"
#include "GeoLayerTypes.h"
#include "GeoArcLayerActor.generated.h"

class UGeoDataSubsystem;
class UInstancedStaticMeshComponent;
class UMaterialInstanceDynamic;

USTRUCT()
struct FRenderedGeoArc
{
    GENERATED_BODY()
    FGeoMessageEnvelope Message;
    FDateTime AddedUtc;
    double SpawnTimeSeconds = 0.0;
    int32 PaletteIndex = 0;
    // Per-end congestion dimming (1 = isolated). Only the segments closest
    // to a congested endpoint fade, so the path keeps its color while the
    // convergence zone stops blowing out white.
    float BrightnessFrom = 1.0f;
    float BrightnessTo = 1.0f;
};

// One palette class of the active traffic window, for instrument panels.
USTRUCT()
struct FGeoPaletteBreakdownEntry
{
    GENERATED_BODY()
    int32 PaletteIndex = 0;
    FString Label;
    FLinearColor Color = FLinearColor::White;
    int32 Count = 0;
};

UCLASS()
class IONCOMMANDVISUALIZATION_API AGeoArcLayerActor : public AActor, public IGeoRenderAdapter
{
    GENERATED_BODY()

public:
    AGeoArcLayerActor();
    virtual void OnConstruction(const FTransform& Transform) override;
    virtual void PostRegisterAllComponents() override;
    virtual void BeginPlay() override;
    virtual void EndPlay(const EEndPlayReason::Type EndPlayReason) override;
    virtual void Tick(float DeltaSeconds) override;

    virtual bool Supports(const FGeoMessageEnvelope& Message) const override;
    virtual void Submit(const FGeoMessageEnvelope& Message) override;
    virtual void Reset() override;

    // Performs a bounded CPU pick only when the operator clicks. This avoids
    // persistent collision bodies for the thousands of instanced arc segments.
    bool FindClosestMessageToRay(const FVector& RayOrigin, const FVector& RayDirection, double RayLength, double MaxDistance, FGeoMessageEnvelope& OutMessage) const;

    // Restricts the layer to relationships touching one of the given entity
    // ids (empty clears the filter) and resubmits the active window.
    void SetEntityFilter(const TArray<FString>& EntityIds);
    bool HasEntityFilter() const { return EntityFilter.Num() > 0; }

    // Restricts the layer to messages whose property Key equals Value (empty
    // value clears the filter) and resubmits the active window.
    void SetPropertyFilter(const FString& Key, const FString& Value);
    const FString& GetPropertyFilterValue() const { return PropertyFilterValue; }

    // Shows a single palette (band) exclusively; pressing the same preset
    // again or passing INDEX_NONE restores all bands. Pure visibility, no
    // data churn.
    void SetBandFocus(int32 PaletteIndex);
    int32 GetBandFocus() const { return BandFocusIndex; }

    // Aggregates the active window per palette class for instrument panels.
    // Entries carry the domain label and color; classes without a label are
    // included only while they hold traffic.
    void GetPaletteBreakdown(TArray<FGeoPaletteBreakdownEntry>& OutEntries) const;
    FString GetTrafficPanelTitle() const { return ResolveTrafficPanelTitle(); }

    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double GlobeRadius = 1000.0;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") int32 SegmentsPerArc = 16;
    // Above the steady state of the live firehose (~480/s * 18 s), so the
    // abrupt cap trim only fires on bursts; normal removal is the fully faded
    // age-based expiry.
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") int32 MaxVisibleArcs = 12000;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double LifetimeSeconds = 18.0;
    // Cylinder scale factors; the base mesh is 100 units wide. Thin beams plus
    // bloom read as light, thick ones read as plastic tubes.
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double ArcThickness = 0.02;
    UPROPERTY(EditAnywhere, Category="ION COMMAND|Layer") double SelectedArcThickness = 0.055;

protected:
    virtual int32 ResolvePaletteIndex(const FGeoMessageEnvelope& Message) const;
    virtual FLinearColor ResolvePaletteColor(int32 PaletteIndex) const;
    virtual FString ResolvePaletteLabel(int32 PaletteIndex) const;
    virtual FString ResolveTrafficPanelTitle() const;
    virtual FGeoLayerManifest CreateLayerManifest() const;

private:
    void OnMessageAccepted(const FGeoMessageEnvelope& Message);
    void BuildEditorPreview();
    void AddArcInstances(const FRenderedGeoArc& Arc);
    float SegmentBrightness(const FRenderedGeoArc& Arc, int32 Segment) const;
    void AddArcInstancesTo(UInstancedStaticMeshComponent* Instances, const FGeoMessageEnvelope& Message, double Thickness, bool bWritePathAlpha = false);
    void AppendArcTransforms(TArray<FTransform>& Out, const FGeoMessageEnvelope& Message, double Thickness) const;
    FVector CalculateArcPoint(const FGeoMessageEnvelope& Message, double Alpha) const;
    void RebuildInstances();
    void RefreshSelectionHighlight();
    void ApplySelectionDimming(bool bDim);
    void OnLayerVisibilityChanged(const FString& LayerId, bool bVisible);
    UPROPERTY(VisibleAnywhere) TObjectPtr<USceneComponent> SceneRoot;
    UPROPERTY(VisibleAnywhere) TArray<TObjectPtr<UInstancedStaticMeshComponent>> PaletteMeshes;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UInstancedStaticMeshComponent> SelectionMesh;
    UPROPERTY(VisibleAnywhere) TObjectPtr<UInstancedStaticMeshComponent> EndpointMesh;
    UPROPERTY(Transient) TArray<TObjectPtr<UMaterialInstanceDynamic>> PaletteMaterials;
    UPROPERTY(Transient) TArray<FRenderedGeoArc> ActiveArcs;
    TWeakObjectPtr<UGeoDataSubsystem> DataSubsystem;
    FString HighlightedMessageId;
    TArray<float> PaletteBaseIntensities;
    TArray<FString> EntityFilter;
    FString PropertyFilterKey;
    FString PropertyFilterValue;
    // Decaying per-endpoint arc counts driving the hotspot dimming.
    TMap<FString, float> EndpointDensity;
    int32 BandFocusIndex = INDEX_NONE;
    bool bSelectionDimmed = false;
    bool bNeedsRebuild = false;
    double LastExpiryCheck = 0.0;
};
