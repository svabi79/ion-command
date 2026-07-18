#include "GeoSelectionSubsystem.h"

void UGeoSelectionSubsystem::SelectMessage(const FGeoMessageEnvelope& Message) { Selection = Message; bHasSelection = true; OnSelectionChanged.Broadcast(); }
void UGeoSelectionSubsystem::ClearSelection() { Selection = FGeoMessageEnvelope(); bHasSelection = false; OnSelectionChanged.Broadcast(); }

