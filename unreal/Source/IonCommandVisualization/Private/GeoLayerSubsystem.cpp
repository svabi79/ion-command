#include "GeoLayerSubsystem.h"

bool UGeoLayerSubsystem::RegisterLayer(const FGeoLayerManifest& Manifest)
{
    if (Manifest.LayerId.IsEmpty() || Manifests.Contains(Manifest.LayerId)) return false;
    Manifests.Add(Manifest.LayerId, Manifest);
    Visibility.Add(Manifest.LayerId, Manifest.bDefaultVisibility);
    return true;
}

bool UGeoLayerSubsystem::SetLayerVisible(const FString& LayerId, bool bVisible)
{
    if (!Manifests.Contains(LayerId)) return false;
    Visibility.FindOrAdd(LayerId) = bVisible;
    LayerVisibilityChanged.Broadcast(LayerId, bVisible);
    return true;
}

bool UGeoLayerSubsystem::IsLayerVisible(const FString& LayerId) const
{
    const bool* Value = Visibility.Find(LayerId);
    return Value != nullptr && *Value;
}

TArray<FGeoLayerManifest> UGeoLayerSubsystem::GetLayers() const
{
    TArray<FGeoLayerManifest> Result;
    Manifests.GenerateValueArray(Result);
    return Result;
}
