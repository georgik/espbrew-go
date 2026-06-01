/**
 * BoundingBoxEditor - Canvas-based bounding box editor for device mapping
 *
 * Features:
 * - Draw, select, edit modes
 * - Unified pointer events (mouse + touch)
 * - Normalized coordinates (0-1)
 * - Visual feedback for mapped/unmapped boxes
 * - Resize handles and drag-to-move
 *
 * Events dispatched:
 * - 'boxCreated' - New box drawn
 * - 'boxSelected' - Box selected
 * - 'boxModified' - Box moved/resized
 * - 'boxDeleted' - Box removed
 */

class BoundingBoxEditor {
    /**
     * @param {HTMLCanvasElement} canvas - The canvas element to draw on
     * @param {HTMLImageElement} image - The reference image element
     */
    constructor(canvas, image) {
        this.canvas = canvas;
        this.ctx = canvas.getContext('2d');
        this.image = image;

        // State
        this.boxes = [];
        this.selectedBox = null;
        this.mode = 'select'; // 'draw', 'select', 'edit'
        this.isDragging = false;
        this.isResizing = false;
        this.dragHandle = null;
        this.dragStart = { x: 0, y: 0 };
        this.lastPointer = { x: 0, y: 0 };
        this.doubleTapTimer = null;
        this.doubleTapDistance = 30; // max distance between taps for double-tap

        // Constants
        this.HANDLE_SIZE = 8;
        this.MIN_BOX_SIZE = 20; // pixels

        // Colors
        this.colors = {
            unmapped: {
                border: '#3b82f6',
                fill: 'rgba(59, 130, 246, 0.2)'
            },
            mapped: {
                border: '#22c55e',
                fill: 'rgba(34, 197, 94, 0.2)'
            },
            selected: {
                border: '#f59e0b',
                fill: 'rgba(245, 158, 11, 0.2)'
            },
            handle: '#f59e0b',
            handleFill: '#fff'
        };

        // Bind methods
        this._handlePointerDown = this._handlePointerDown.bind(this);
        this._handlePointerMove = this._handlePointerMove.bind(this);
        this._handlePointerUp = this._handlePointerUp.bind(this);
        this._handleKeyDown = this._handleKeyDown.bind(this);
        this._handleImageLoad = this._handleImageLoad.bind(this);

        // Setup
        this._setupEventListeners();
    }

    /**
     * Load a reference image
     * @param {string} src - Image URL
     */
    async loadImage(src) {
        return new Promise((resolve, reject) => {
            this.image.onload = () => {
                this._resizeCanvas();
                this._render();
                resolve();
            };
            this.image.onerror = reject;
            this.image.src = src;
        });
    }

    /**
     * Set editor mode
     * @param {string} mode - 'draw', 'select', or 'edit'
     */
    setMode(mode) {
        if (!['draw', 'select', 'edit'].includes(mode)) {
            throw new Error('Invalid mode. Use: draw, select, or edit');
        }
        this.mode = mode;
        this.selectedBox = null;
        this._render();
    }

    /**
     * Get current boxes as normalized coordinates
     * @returns {Array} Array of box objects with normalized coordinates
     */
    getBoxes() {
        return this.boxes.map(box => ({
            id: box.id,
            deviceId: box.deviceId || null,
            bounds: this._pixelsToNormalized(box)
        }));
    }

    /**
     * Load existing boxes from normalized coordinates
     * @param {Array} boxes - Array of box objects with normalized bounds
     */
    setBoxes(boxes) {
        this.boxes = boxes.map(box => ({
            id: box.id,
            deviceId: box.deviceId || null,
            ...this._normalizedToPixels(box.bounds)
        }));
        this.selectedBox = null;
        this._render();
    }

    /**
     * Clear all boxes
     */
    clear() {
        this.boxes = [];
        this.selectedBox = null;
        this._render();
    }

    /**
     * Cleanup event listeners
     */
    destroy() {
        this.canvas.removeEventListener('pointerdown', this._handlePointerDown);
        this.canvas.removeEventListener('pointermove', this._handlePointerMove);
        this.canvas.removeEventListener('pointerup', this._handlePointerUp);
        this.canvas.removeEventListener('pointerleave', this._handlePointerUp);
        document.removeEventListener('keydown', this._handleKeyDown);
        this.image.removeEventListener('load', this._handleImageLoad);
    }

    /**
     * Setup event listeners
     * @private
     */
    _setupEventListeners() {
        // Pointer events (unified mouse + touch)
        this.canvas.addEventListener('pointerdown', this._handlePointerDown);
        this.canvas.addEventListener('pointermove', this._handlePointerMove);
        this.canvas.addEventListener('pointerup', this._handlePointerUp);
        this.canvas.addEventListener('pointerleave', this._handlePointerUp);

        // Keyboard events
        document.addEventListener('keydown', this._handleKeyDown);

        // Image load
        this.image.addEventListener('load', this._handleImageLoad);
    }

    /**
     * Resize canvas to match image display size
     * @private
     */
    _resizeCanvas() {
        const rect = this.image.getBoundingClientRect();
        this.canvas.width = rect.width;
        this.canvas.height = rect.height;
        this.canvas.style.width = rect.width + 'px';
        this.canvas.style.height = rect.height + 'px';
    }

    /**
     * Handle image load
     * @private
     */
    _handleImageLoad() {
        this._resizeCanvas();
        this._render();
    }

    /**
     * Handle pointer down
     * @private
     */
    _handlePointerDown(event) {
        event.preventDefault();
        this.canvas.setPointerCapture(event.pointerId);

        const point = this._getCanvasPoint(event);
        this.lastPointer = { ...point };
        this.isDragging = true;

        switch (this.mode) {
            case 'draw':
                this._startDrawing(point);
                break;
            case 'select':
            case 'edit':
                this._handleSelectOrEdit(point);
                break;
        }
    }

    /**
     * Handle pointer move
     * @private
     */
    _handlePointerMove(event) {
        event.preventDefault();
        if (!this.isDragging) {
            // Update cursor based on hover
            const point = this._getCanvasPoint(event);
            this._updateCursor(point);
            return;
        }

        const point = this._getCanvasPoint(event);
        const delta = {
            x: point.x - this.lastPointer.x,
            y: point.y - this.lastPointer.y
        };

        switch (this.mode) {
            case 'draw':
                if (this.selectedBox) {
                    this._updateDrawingBox(point);
                }
                break;
            case 'edit':
                if (this.isResizing && this.selectedBox && this.dragHandle) {
                    this._resizeBox(delta, this.dragHandle);
                } else if (this.selectedBox && !this.isResizing) {
                    this._moveBox(delta);
                }
                break;
        }

        this.lastPointer = { ...point };
        this._render();
    }

    /**
     * Handle pointer up
     * @private
     */
    _handlePointerUp(event) {
        event.preventDefault();
        this.canvas.releasePointerCapture(event.pointerId);
        this.isDragging = false;
        this.isResizing = false;
        this.dragHandle = null;

        // Check for double-tap
        if (this.mode !== 'draw') {
            this._checkDoubleTap(event);
        }

        // Finalize drawn box
        if (this.mode === 'draw' && this.selectedBox) {
            const box = this.selectedBox;
            // Ensure minimum size
            if (box.width < this.MIN_BOX_SIZE || box.height < this.MIN_BOX_SIZE) {
                this.boxes = this.boxes.filter(b => b.id !== box.id);
                this.selectedBox = null;
            } else {
                this._dispatchEvent('boxCreated', { box: this._getBoxData(box) });
                this.selectedBox = null;
                this._render();
            }
        }

        this._render();
    }

    /**
     * Handle keyboard events
     * @private
     */
    _handleKeyDown(event) {
        if (event.key === 'Delete' || event.key === 'Backspace') {
            if (this.selectedBox && document.activeElement.tagName !== 'INPUT') {
                event.preventDefault();
                this._deleteBox(this.selectedBox);
            }
        }
    }

    /**
     * Start drawing a new box
     * @private
     */
    _startDrawing(point) {
        const newBox = {
            id: 'box-' + Date.now(),
            deviceId: null,
            x: point.x,
            y: point.y,
            width: 0,
            height: 0
        };
        this.boxes.push(newBox);
        this.selectedBox = newBox;
        this._render();
    }

    /**
     * Update box being drawn
     * @private
     */
    _updateDrawingBox(point) {
        if (!this.selectedBox) return;

        const box = this.selectedBox;
        const start = { x: box.x, y: box.y };

        // Handle negative width/height (drawing backwards)
        box.width = Math.abs(point.x - start.x);
        box.height = Math.abs(point.y - start.y);
        box.x = Math.min(point.x, start.x);
        box.y = Math.min(point.y, start.y);

        // Clamp to canvas bounds
        this._clampBox(box);
    }

    /**
     * Handle select or edit mode interactions
     * @private
     */
    _handleSelectOrEdit(point) {
        // Check if clicking on a resize handle
        if (this.mode === 'edit' && this.selectedBox) {
            const handle = this._getHandleAtPoint(point);
            if (handle) {
                this.isResizing = true;
                this.dragHandle = handle;
                this.dragStart = { ...point };
                return;
            }
        }

        // Check if clicking on a box
        const box = this._getBoxAtPoint(point);
        if (box) {
            this.selectedBox = box;
            this.dragStart = { ...point };
            this._dispatchEvent('boxSelected', { box: this._getBoxData(box) });
        } else {
            this.selectedBox = null;
        }

        this._render();
    }

    /**
     * Move selected box
     * @private
     */
    _moveBox(delta) {
        if (!this.selectedBox) return;

        this.selectedBox.x += delta.x;
        this.selectedBox.y += delta.y;

        this._clampBox(this.selectedBox);
        this._dispatchEvent('boxModified', { box: this._getBoxData(this.selectedBox) });
    }

    /**
     * Resize box by handle
     * @private
     */
    _resizeBox(delta, handle) {
        if (!this.selectedBox) return;

        const box = this.selectedBox;

        switch (handle) {
            case 'nw':
                box.x += delta.x;
                box.y += delta.y;
                box.width -= delta.x;
                box.height -= delta.y;
                break;
            case 'ne':
                box.y += delta.y;
                box.width += delta.x;
                box.height -= delta.y;
                break;
            case 'sw':
                box.x += delta.x;
                box.width -= delta.x;
                box.height += delta.y;
                break;
            case 'se':
                box.width += delta.x;
                box.height += delta.y;
                break;
        }

        // Handle negative size (flipped)
        if (box.width < 0) {
            box.x += box.width;
            box.width = Math.abs(box.width);
        }
        if (box.height < 0) {
            box.y += box.height;
            box.height = Math.abs(box.height);
        }

        this._clampBox(box);
        this._dispatchEvent('boxModified', { box: this._getBoxData(box) });
    }

    /**
     * Delete a box
     * @private
     */
    _deleteBox(box) {
        this.boxes = this.boxes.filter(b => b.id !== box.id);
        this.selectedBox = null;
        this._dispatchEvent('boxDeleted', { boxId: box.id });
        this._render();
    }

    /**
     * Check for double-tap gesture
     * @private
     */
    _checkDoubleTap(event) {
        if (this.doubleTapTimer) {
            const lastTap = this.doubleTapTimer;
            this.doubleTapTimer = null;
            clearTimeout(lastTap.timeout);

            const point = this._getCanvasPoint(event);
            const distance = Math.sqrt(
                Math.pow(point.x - lastTap.x, 2) +
                Math.pow(point.y - lastTap.y, 2)
            );

            if (distance < this.doubleTapDistance) {
                // Double-tap detected - delete tapped box
                const box = this._getBoxAtPoint(point);
                if (box) {
                    this._deleteBox(box);
                }
            }
        } else {
            const point = this._getCanvasPoint(event);
            this.doubleTapTimer = {
                x: point.x,
                y: point.y,
                timeout: setTimeout(() => {
                    this.doubleTapTimer = null;
                }, 300)
            };
        }
    }

    /**
     * Update cursor based on hover
     * @private
     */
    _updateCursor(point) {
        if (this.mode === 'draw') {
            this.canvas.style.cursor = 'crosshair';
            return;
        }

        // Check handles first (edit mode only)
        if (this.mode === 'edit' && this.selectedBox) {
            const handle = this._getHandleAtPoint(point);
            if (handle) {
                const cursors = {
                    nw: 'nw-resize',
                    ne: 'ne-resize',
                    sw: 'sw-resize',
                    se: 'se-resize'
                };
                this.canvas.style.cursor = cursors[handle];
                return;
            }
        }

        // Check if over a box
        const box = this._getBoxAtPoint(point);
        if (box) {
            this.canvas.style.cursor = this.mode === 'edit' ? 'move' : 'pointer';
        } else {
            this.canvas.style.cursor = 'default';
        }
    }

    /**
     * Get box at point
     * @private
     */
    _getBoxAtPoint(point) {
        // Search in reverse order (topmost first)
        for (let i = this.boxes.length - 1; i >= 0; i--) {
            const box = this.boxes[i];
            if (point.x >= box.x && point.x <= box.x + box.width &&
                point.y >= box.y && point.y <= box.y + box.height) {
                return box;
            }
        }
        return null;
    }

    /**
     * Get resize handle at point
     * @private
     */
    _getHandleAtPoint(point) {
        if (!this.selectedBox) return null;

        const box = this.selectedBox;
        const handles = [
            { name: 'nw', x: box.x, y: box.y },
            { name: 'ne', x: box.x + box.width, y: box.y },
            { name: 'sw', x: box.x, y: box.y + box.height },
            { name: 'se', x: box.x + box.width, y: box.y + box.height }
        ];

        const hitRadius = this.HANDLE_SIZE + 4;

        for (const handle of handles) {
            const distance = Math.sqrt(
                Math.pow(point.x - handle.x, 2) +
                Math.pow(point.y - handle.y, 2)
            );
            if (distance <= hitRadius) {
                return handle.name;
            }
        }

        return null;
    }

    /**
     * Clamp box to canvas bounds
     * @private
     */
    _clampBox(box) {
        box.x = Math.max(0, Math.min(box.x, this.canvas.width - box.width));
        box.y = Math.max(0, Math.min(box.y, this.canvas.height - box.height));
        box.width = Math.max(this.MIN_BOX_SIZE, Math.min(box.width, this.canvas.width - box.x));
        box.height = Math.max(this.MIN_BOX_SIZE, Math.min(box.height, this.canvas.height - box.y));
    }

    /**
     * Convert pixel coordinates to normalized (0-1)
     * @private
     */
    _pixelsToNormalized(box) {
        return {
            x: box.x / this.canvas.width,
            y: box.y / this.canvas.height,
            width: box.width / this.canvas.width,
            height: box.height / this.canvas.height
        };
    }

    /**
     * Convert normalized (0-1) coordinates to pixels
     * @private
     */
    _normalizedToPixels(bounds) {
        return {
            x: bounds.x * this.canvas.width,
            y: bounds.y * this.canvas.height,
            width: bounds.width * this.canvas.width,
            height: bounds.height * this.canvas.height
        };
    }

    /**
     * Get point relative to canvas
     * @private
     */
    _getCanvasPoint(event) {
        const rect = this.canvas.getBoundingClientRect();
        return {
            x: event.clientX - rect.left,
            y: event.clientY - rect.top
        };
    }

    /**
     * Get box data for events
     * @private
     */
    _getBoxData(box) {
        return {
            id: box.id,
            deviceId: box.deviceId,
            bounds: this._pixelsToNormalized(box)
        };
    }

    /**
     * Dispatch custom event
     * @private
     */
    _dispatchEvent(type, detail) {
        const event = new CustomEvent(type, { detail });
        this.canvas.dispatchEvent(event);
    }

    /**
     * Render the canvas
     * @private
     */
    _render() {
        const ctx = this.ctx;
        ctx.clearRect(0, 0, this.canvas.width, this.canvas.height);

        // Draw all boxes
        for (const box of this.boxes) {
            this._renderBox(box);
        }

        // Draw resize handles for selected box (edit mode only)
        if (this.selectedBox && this.mode === 'edit') {
            this._renderHandles(this.selectedBox);
        }
    }

    /**
     * Render a single box
     * @private
     */
    _renderBox(box) {
        const ctx = this.ctx;
        const isSelected = this.selectedBox && this.selectedBox.id === box.id;
        const isMapped = !!box.deviceId;

        // Choose colors
        let colors;
        if (isSelected) {
            colors = this.colors.selected;
        } else if (isMapped) {
            colors = this.colors.mapped;
        } else {
            colors = this.colors.unmapped;
        }

        // Draw fill
        ctx.fillStyle = colors.fill;
        ctx.fillRect(box.x, box.y, box.width, box.height);

        // Draw border
        ctx.strokeStyle = colors.border;
        ctx.lineWidth = isSelected ? 3 : 2;
        ctx.strokeRect(box.x, box.y, box.width, box.height);

        // Draw device label for mapped boxes
        if (isMapped && box.deviceId) {
            this._renderLabel(box);
        }
    }

    /**
     * Render device label on box
     * @private
     */
    _renderLabel(box) {
        const ctx = this.ctx;
        const label = box.deviceId.substring(0, 12);

        ctx.font = '11px monospace';
        const textWidth = ctx.measureText(label).width;
        const padding = 4;

        // Draw label background
        ctx.fillStyle = this.colors.mapped.border;
        ctx.fillRect(
            box.x,
            box.y - 18,
            textWidth + padding * 2,
            18
        );

        // Draw label text
        ctx.fillStyle = '#fff';
        ctx.textBaseline = 'middle';
        ctx.fillText(label, box.x + padding, box.y - 9);
    }

    /**
     * Render resize handles
     * @private
     */
    _renderHandles(box) {
        const ctx = this.ctx;
        const handles = [
            { x: box.x, y: box.y },              // NW
            { x: box.x + box.width, y: box.y },  // NE
            { x: box.x, y: box.y + box.height }, // SW
            { x: box.x + box.width, y: box.y + box.height } // SE
        ];

        for (const handle of handles) {
            // Draw handle
            ctx.fillStyle = this.colors.handle;
            ctx.beginPath();
            ctx.arc(handle.x, handle.y, this.HANDLE_SIZE, 0, Math.PI * 2);
            ctx.fill();

            // Draw handle center
            ctx.fillStyle = this.colors.handleFill;
            ctx.beginPath();
            ctx.arc(handle.x, handle.y, this.HANDLE_SIZE / 2, 0, Math.PI * 2);
            ctx.fill();
        }
    }
}

// Export for use in other modules
if (typeof module !== 'undefined' && module.exports) {
    module.exports = BoundingBoxEditor;
}
